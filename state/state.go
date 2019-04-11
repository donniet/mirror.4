package state

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

var podKinds map[reflect.Kind]bool

func init() {
	podKinds = map[reflect.Kind]bool{
		reflect.Bool:       true,
		reflect.Int:        true,
		reflect.Int8:       true,
		reflect.Int16:      true,
		reflect.Int32:      true,
		reflect.Int64:      true,
		reflect.Uint:       true,
		reflect.Uint8:      true,
		reflect.Uint16:     true,
		reflect.Uint32:     true,
		reflect.Uint64:     true,
		reflect.Complex64:  true,
		reflect.Complex128: true,
		reflect.String:     true,
	}
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

type Server struct {
	Data       interface{}
	fieldCache map[reflect.Type]map[string]int
}

func NewServer(dat interface{}) *Server {
	return &Server{Data: dat}
}

func (s *Server) fieldIndexByName(t reflect.Type, name string) int {
	if t.Kind() != reflect.Struct {
		return -1
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
		return dex
	}

	return -1
}

type Statuser interface {
	Status() int
}

type NotFoundError string

func (e NotFoundError) Status() int   { return http.StatusNotFound }
func (e NotFoundError) Error() string { return string(e) }

type InternalServerError string

func (e InternalServerError) Status() int   { return http.StatusInternalServerError }
func (e InternalServerError) Error() string { return string(e) }

type BadRequestError string

func (e BadRequestError) Status() int   { return http.StatusBadRequest }
func (e BadRequestError) Error() string { return string(e) }

func (s *Server) nextValue(v reflect.Value, path string) (child reflect.Value, rest string, err error) {
	if v == (reflect.Value{}) {
		err = fmt.Errorf("empty value")
		return
	}

	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	first := ""
	first, rest = chompPath(path)

	if first == "" {
		err = fmt.Errorf("empty path")
		return
	}

	dex := 0
	if v.Kind() == reflect.Struct {
		dex = s.fieldIndexByName(v.Type(), first)
		if dex < 0 {
			err = fmt.Errorf("field not found")
			return
		}
		child = v.Field(dex)
	} else if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		d := int64(0)
		if d, err = strconv.ParseInt(first, 10, 64); err != nil || d < 0 || d >= int64(v.Len()) {
			err = fmt.Errorf("invalid integer conversion")
			return
		} else {
			dex = int(d)
		}

		child = v.Index(int(dex))
	} else if v.Kind() == reflect.Map {
		if v.Type().Key().Kind() == reflect.String {
			child = v.MapIndex(reflect.ValueOf(first))

			if child == (reflect.Value{}) {
				err = fmt.Errorf("key not found")
				return
			}
		} else {
			// only accept map[string]type for now
			err = fmt.Errorf("only map[string]type allowed")
			return
		}
	} else {
		err = fmt.Errorf("type does not allow elements")
		return
	}

	return
}

func (s *Server) Get(path string) (*json.RawMessage, error) {
	v := reflect.ValueOf(s.Data)
	first := ""
	dex := 0
	p := path

	var ret interface{}

	for v != (reflect.Value{}) {
		for v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		first, p = chompPath(p)

		if first == "" {
			ret = v.Interface()
			break
		}

		if v.Kind() == reflect.Struct {
			dex = s.fieldIndexByName(v.Type(), first)
			if dex < 0 {
				break
			}
			v = v.Field(dex)
		} else if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			if d, err := strconv.ParseInt(first, 10, 64); err != nil || d < 0 || d >= int64(v.Len()) {
				break
			} else {
				dex = int(d)
			}

			v = v.Index(int(dex))
		} else if v.Kind() == reflect.Map {
			if v.Type().Key().Kind() == reflect.String {
				v = v.MapIndex(reflect.ValueOf(first))
			} else {
				// only accept map[string]type for now
				break
			}
		} else {
			break
		}
	}

	if ret == nil {
		return nil, NotFoundError(fmt.Sprintf("'%s' not found", path))
	}

	if b, err := json.Marshal(ret); err != nil {
		return nil, InternalServerError(err.Error())
	} else {
		return (*json.RawMessage)(&b), nil
	}
}
func (s *Server) Post(path string, body *json.RawMessage) (string, error) {
	v := reflect.ValueOf(s.Data)
	p := path
	first := ""
	dex := -1

	if body == nil || len(*body) == 0 {
		return "", BadRequestError("body is empty")
	}

	for v != (reflect.Value{}) {
		var w reflect.Value

		for v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		first, p = chompPath(p)

		if first == "" {
			break
		}

		if v.Kind() == reflect.Struct {
			dex = s.fieldIndexByName(v.Type(), first)
			if dex < 0 {
				break
			}
			w = v.Field(dex)
		} else if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			d, err := strconv.ParseInt(first, 10, 64)
			if err != nil || d < 0 || d >= int64(v.Len()) {
				break
			}
			dex = int(d)
			w = v.Index(dex)
		} else if v.Kind() == reflect.Map {
			if v.Type().Key().Kind() == reflect.String {
				w = v.MapIndex(reflect.ValueOf(first))
			} else {
				break
			}
		}

		if w == (reflect.Value{}) {
			break
		}

		if p != "" {
			continue
		}

		if w.Kind() != reflect.Ptr {
			if !w.CanAddr() {
				break
			}
			w = w.Addr()
		}

		if !w.CanInterface() {
			break
		}

		if err := json.Unmarshal(*body, w.Interface()); err != nil {
			return "", InternalServerError(err.Error())
		}
		return path, nil
	}

	return "", NotFoundError(fmt.Sprintf("'%s' not found", path))
}
func (s *Server) Put(path string, body *json.RawMessage) (string, error) {
	return "", nil
}
func (s *Server) Delete(path string) error {
	return nil
}
