package state

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Getter interface {
	Get(path string) ([]byte, error)
}
type Putter interface {
	Put(path string, body []byte) (string, error)
}
type Poster interface {
	Post(path string, body []byte) (string, error)
}
type Deleter interface {
	Delete(path string) error
}

func chompPath(path string) (string, string) {
	if len(path) == 0 || path == "/" {
		return "", ""
	}

	if path[0:1] == "/" {
		path = path[1:]
	}

	slash := strings.Index(path, "/")

	if slash < 0 {
		return path, ""
	}

	return path[0:slash], path[slash+1:]
}

// Server wraps an interface and adds Get, Put, Post and Delete methods
type Server struct {
	Data       interface{}
	fieldCache map[reflect.Type]map[string]int
	locker     sync.Locker
}

// DoLocked executes the task function while locked
func (s *Server) DoLocked(task func()) {
	s.locker.Lock()
	defer s.locker.Unlock()

	task()
}

// NewServer creates a new server from an interface{}
func NewServer(dat interface{}) *Server {
	return &Server{Data: dat, locker: new(sync.Mutex)}
}

func (s *Server) fieldIndexByName(t reflect.Type, name string) (int, reflect.StructTag) {
	if t.Kind() != reflect.Struct {
		return -1, ""
	}

	if s.fieldCache == nil {
		s.fieldCache = make(map[reflect.Type]map[string]int)
	}

	var cache map[string]int

	if cache = s.fieldCache[t]; cache == nil {
		cache = make(map[string]int)

		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)

			// only exported fields
			if len(f.Name) == 0 || strings.ToUpper(f.Name[0:1]) != f.Name[0:1] {
				continue
			}

			json, _ := f.Tag.Lookup("json")

			if json == "" {
				cache[f.Name] = i
				continue
			}

			comma := strings.Index(json, ",")
			if comma < 0 {
				comma = len(json)
			}

			cache[json[0:comma]] = i
		}

		s.fieldCache[t] = cache
	}

	if dex, ok := cache[name]; ok {
		return dex, t.Field(dex).Tag
	}

	return -1, ""
}

// Statuser returns a status compatible with http.Status* messages
type Statuser interface {
	error
	Status() int
}

// NotFoundError returns a 404 status
type NotFoundError string

// Status returns http.StatusNotFound
func (e NotFoundError) Status() int { return http.StatusNotFound }

// Error returns an error message compatible with error
func (e NotFoundError) Error() string { return string(e) }

// InternalServerError returns a 500 status
type InternalServerError string

// Status returns http.StatusInternalServerError
func (e InternalServerError) Status() int { return http.StatusInternalServerError }

// Error returns an error message compatible with error
func (e InternalServerError) Error() string { return string(e) }

// BadRequestError returns a 400 status
type BadRequestError string

// Status returns an http.StatusBadRequest
func (e BadRequestError) Status() int { return http.StatusBadRequest }

// Error returns an error message compatible with error
func (e BadRequestError) Error() string { return string(e) }

func (s *Server) nextValue(v reflect.Value, path string) (child reflect.Value, rest string, tag reflect.StructTag, err error) {
	if v == (reflect.Value{}) {
		err = InternalServerError("empty value")
		return
	}

	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	first := ""
	first, rest = chompPath(path)

	if first == "" {
		return v, "", "", nil
	}

	dex := 0
	if v.Kind() == reflect.Struct {
		// log.Printf("looking for field '%s' in type '%v'", first, v.Type())
		dex, tag = s.fieldIndexByName(v.Type(), first)
		if dex < 0 {
			err = NotFoundError("field not found")
			return
		}
		child = v.Field(dex)
	} else if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		d := int64(0)
		if d, err = strconv.ParseInt(first, 10, 64); err != nil || d < 0 || d >= int64(v.Len()) {
			err = BadRequestError("invalid integer conversion")
			return
		} else {
			dex = int(d)
		}

		child = v.Index(int(dex))
	} else if v.Kind() == reflect.Map {
		if v.Type().Key().Kind() == reflect.String {
			child = v.MapIndex(reflect.ValueOf(first))

			if child == (reflect.Value{}) {
				err = NotFoundError("key not found")
				return
			}
		} else {
			// only accept map[string]type for now
			err = BadRequestError("only map[string]type allowed")
			return
		}
	} else {
		err = BadRequestError("type does not allow elements")
		return
	}

	return
}

// Get takes a '/' seperated path and dives into the wrapped interface
func (s *Server) Get(path string) ([]byte, error) {
	// this is slow for now, we'll speed it up later
	s.locker.Lock()
	defer s.locker.Unlock()

	v := reflect.ValueOf(s.Data)
	rest := path
	var err error
	var ret interface{}

	// log.Printf("path: %s", path)

	for v != (reflect.Value{}) {
		v, rest, _, err = s.nextValue(v, rest)

		// log.Printf("rest: %s", rest)
		if err != nil {
			return nil, err
		}

		if rest == "" {
			ret = v.Interface()
			break
		}
	}

	if ret == nil {
		return nil, NotFoundError(fmt.Sprintf("'%s' not found", path))
	}

	if b, err := json.Marshal(ret); err != nil {
		return nil, InternalServerError(err.Error())
	} else {
		return b, nil
	}
}

// Post allows modification of a field in the wrapped interface
func (s *Server) Post(path string, body []byte) (string, error) {
	// this is slow for now, we'll speed it up later
	s.locker.Lock()
	defer s.locker.Unlock()

	v := reflect.ValueOf(s.Data)
	rest := path
	var err error

	notFound := NotFoundError(fmt.Sprintf("'%s' not found", path))

	if v == (reflect.Value{}) {
		return "", notFound
	}

	for rest != "" {
		v, rest, _, err = s.nextValue(v, rest)
		if err != nil {
			return "", err
		}

		if v == (reflect.Value{}) {
			return "", notFound
		}
	}

	if v.Kind() != reflect.Ptr {
		if !v.CanAddr() {
			return "", notFound
		}
		v = v.Addr()
	}
	if !v.CanInterface() {
		return "", notFound
	}
	if err := json.Unmarshal(body, v.Interface()); err != nil {
		return "", InternalServerError(err.Error())
	}
	return path, nil
}

type apiTag string

func (a apiTag) Maximum() int {
	s := string(a)

	r := regexp.MustCompile("maximum=(\\d+)")

	matches := r.FindStringSubmatch(s)

	if len(matches) != 2 {
		return math.MaxInt32
	}

	max, _ := strconv.ParseInt(matches[1], 10, 32)
	return int(max)

}

// Put adds a new element to map or slice
func (s *Server) Put(path string, body []byte) (string, error) {
	// this is slow for now, we'll speed it up later
	s.locker.Lock()
	defer s.locker.Unlock()

	v := reflect.ValueOf(s.Data)
	rest := path
	tag := reflect.StructTag("")
	var err error

	notFound := NotFoundError(fmt.Sprintf("'%s' not found", path))

	if v == (reflect.Value{}) {
		return "", notFound
	}

	for {
		if v.Kind() == reflect.Map && strings.Index(rest, "/") < 0 {
			break
		} else if rest == "" {
			break
		}

		v, rest, tag, err = s.nextValue(v, rest)
		if err != nil {
			return "", err
		}

		if v == (reflect.Value{}) {
			return "", notFound
		}
	}

	if v.Kind() != reflect.Map && v.Kind() != reflect.Slice {
		return "", BadRequestError("not allowed")
	}

	el := v.Type().Elem()
	var n reflect.Value

	indirect := false
	if el.Kind() == reflect.Ptr {
		el = el.Elem()
		indirect = true
	}
	n = reflect.New(el)

	if err := json.Unmarshal(body, n.Interface()); err != nil {
		return "", BadRequestError(err.Error())
	}

	// log.Printf("v.Kind() == %v", v.Kind())
	if v.Kind() == reflect.Map {
		// add to the key
		if indirect {
			v.SetMapIndex(reflect.ValueOf(rest), n)
		} else {
			v.SetMapIndex(reflect.ValueOf(rest), n.Elem())
		}
		return path, nil
	} else if v.Kind() == reflect.Slice {
		// append
		max := math.MaxInt32

		if t, ok := tag.Lookup("api"); ok {
			a := apiTag(t)

			max = a.Maximum()
		}

		if v.Len() >= max {
			v.Set(v.Slice(v.Len()-max+1, v.Len()))
		}

		if indirect {
			v.Set(reflect.Append(v, n))
		} else {
			v.Set(reflect.Append(v, n.Elem()))
		}
		rest = fmt.Sprintf("%d", v.Len()-1)
		return path + "/" + rest, nil
	}

	return "", BadRequestError("path not map or slice")
}

// Delete removes an item from a slice or map
func (s *Server) Delete(path string) error {
	// this is slow for now, we'll speed it up later
	s.locker.Lock()
	defer s.locker.Unlock()

	v := reflect.ValueOf(s.Data)
	rest := path
	var err error

	notFound := NotFoundError(fmt.Sprintf("'%s' not found", path))

	if v.Kind() == reflect.Invalid {
		return notFound
	}

	for {
		if strings.Index(rest, "/") < 0 {
			break
		}

		v, rest, _, err = s.nextValue(v, rest)
		if err != nil {
			return err
		}

		if v == (reflect.Value{}) {
			return notFound
		}
	}

	if v.Kind() == reflect.Map {
		// nil set
		d := v.MapIndex(reflect.ValueOf(rest))
		if d.Kind() == reflect.Invalid {
			return NotFoundError(fmt.Sprintf("key not found '%s'", rest))
		}
		v.SetMapIndex(reflect.ValueOf(rest), reflect.Value{})
	} else if v.Kind() == reflect.Slice {
		if n, err := strconv.ParseInt(rest, 10, 64); err != nil {
			return BadRequestError(err.Error())
		} else if i := int(n); i < 0 || i >= v.Len() {
			return NotFoundError(fmt.Sprintf("index '%d' out of range", i))
		} else if !v.CanSet() {
			return InternalServerError("cannot set slice")
		} else {
			v.Set(reflect.AppendSlice(v.Slice(0, i), v.Slice(i+1, v.Len())))
		}
	} else {
		return BadRequestError(fmt.Sprintf("cannot delete from type %v", v.Kind()))
	}

	return nil
}
