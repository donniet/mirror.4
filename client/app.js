function loadAsync(url) {
    return new Promise(function (resolve, reject) {
        let xhr = new XMLHttpRequest();
        xhr.onreadystatechange = function(evt) {
            if (xhr.readyState == 4) {
                if (xhr.status == 200) {
                    resolve(xhr.responseText);
                } else {
                    resolve(null);
                }
            }
        }
        xhr.open('GET', url, true);
        xhr.send();
    });
}

// loadAsync('client/icons/clear-day.svg').then(function(svg) {
//   svg = svg.replace(/<\?.*>\s*/g, '');
//   svg = svg.replace(/<!.*>\s*/g, '');
//   console.log(svg);

//   let div = document.createElement('div');
//   div.innerHTML = svg;

//   document.body.appendChild(div);
// });

function App(websocketUrl, el) {
    this.websocketUrl = websocketUrl;
    this.el = el;
    this.app = null;
    this.checkStreams = new Object();
    this.open();
}
App.prototype.setResponse = function(response) {
    if (this.app) {
        console.log('setting response');
        this.app.response = response;
        return;
    }
    console.log('creating app');
    this.app = new Vue({
        el: this.el,
        data: function() {
            return {
                response: response,
            };
        },
        methods: {
            "formatTime": formatTime,
            "markError": App.prototype.markStreamError.bind(this),
            "markLoaded": App.prototype.markStreamLoaded.bind(this),
        },
        computed: {
            recentPeople: function() {
                console.log('filtered people');
                let people = this.response.faces.detections;

                detections = people.filter((p) => {
                    return (new Date()).getTime() - Date.parse(p.dateTime) < 1000 * 60 * 5; // 5 min
                });

                p = {}
                ret = [];
                for (var d in detections) {
                    if (!p[d.name]) {
                        p[d.name] = true;
                        ret.push(d);
                    }
                }

                return ret;
            }
        }
    });
};
App.prototype.markStreamLoaded = function(index) {
    this.sendMessage({
        'method': 'POST',
        'path': `streams/${index}/errorTime`,
        'body': "0001-01-01T00:00:00Z"
    });
    if(this.checkStreams[index]) {
        delete this.checkStreams[index];
    }
};
App.prototype.markStreamError = function(index) {
    this.sendMessage({
        'method': 'POST',
        'path': `streams/${index}/errorTime`,
        'body': new Date(),
    });
    
    if(!this.checkStreams[index]) {
        this.checkStreams[index] = this.app.response.streams[index];
    }
}
App.prototype.sendMessage = function(msg) {
    this.ws.send(JSON.stringify(msg));
}
App.prototype.open = function() {
    this.ws = new WebSocket(this.websocketUrl);
    this.ws.onopen = App.prototype.onopen.bind(this);
};
App.prototype.onopen = function(e) {
    console.log('websocket opened');
    this.ws.onmessage = App.prototype.onmessage.bind(this);
    this.ws.onerror = App.prototype.onerror.bind(this);
    this.ws.onclose = App.prototype.onclose.bind(this);

    this.sendRequest('GET', '/');
};
function postHelper(data, path, body) {
    let slash = -1;
    while((slash = path.indexOf('/')) >= 0) {
        let first = path.substring(0, slash);
        path = path.substring(slash + 1);

        if (first == "") continue;

        if (data[first]) {
            data = data[first];
        } else {
            console.log('could not find path', path, data);
            return;
        }

        console.log(first, path);
    }

    data[path] = body;
}
App.prototype.handlePost = function(path, body) {
    console.log('post', path, body);
    postHelper(this.app.response, path, body);
};
function putHelper(data, path, body) {
    while(path != "") {
        let slash = path.indexOf('/');
        if (slash >= 0) {
            first = path.substring(0, slash);
            path = path.substring(slash + 1);
        } else if (typeof data == 'object' && typeof data[first] == 'object' && data[first].length != 'number') {
            console.log('overwriting', first, data, body);
            data[first] = body;
            return;
        } else {
            first = path;
            path = "";
        }
        // ignore double slashes
        if (first == "") continue;

        if (!data[first]) {
            console.log('could not find path', first, data);
            return;
        }

        data = data[first];
    }

    if (typeof data == 'object' && typeof data.length == 'number') {
        data.push(body);
    } else {
        console.log("don't know how to put", data, body);
        return;
    }
}
App.prototype.handlePut = function(path, body) {
    console.log('put', path, body);
    putHelper(this.app.response, path, body);
};
function deleteHelper(data, path) {
    let slash = -1;
    while((slash = path.indexOf('/')) >= 0) {
        let first = path.substring(0, slash);
        path = path.substring(slash + 1);

        // ignore double slashes
        if(first == "") continue;

        if(!data[first]) {
            console.log('could not find path', first, data);
            break;
        }

        data = data[first];
    }

    if (typeof data != 'object') {
        console.log('cannot delete', from, data);
        return;
    }

    if (typeof data.length == 'number') {
        let index = parseInt(path, 10);
        if (isNaN(index) || index < 0 || index >= data.length) {
            console.log('could not delete', path, data);
            return;
        }
        console.log('splicing from array', index, data);
        data.splice(index, 1);
    } else {
        console.log('deleting from object', path, data);
        delete data[path];
    }
}
App.prototype.handleDelete = function(path) {
    console.log('delete', path);
    deleteHelper(this.app.response, path);
};
App.prototype.onmessage = function(e) {
    if (!e.data) return;

    var dat = JSON.parse(e.data);

    console.log('message', dat, dat.method);

    if (dat.error) {
        console.log('error received: ', dat.error)
        return;
    }

    if (!dat.method) {
        this.setResponse(dat);
        return;
    }

    switch (dat.method) {
    case "POST":
        this.handlePost(dat.path, dat.body);
        break;
    case "PUT": 
        this.handlePut(dat.path, dat.body);
        break;
    case "DELETE": 
        this.handleDelete(dat.path);
        break;
    default:
        return;
    }
};
App.prototype.onerror = function(e) {
    console.log('error', e);
};
App.prototype.onclose = function(e) {
    setTimeout(function() { this.open(); }.bind(this), 1000);
};
App.prototype.sendRequest = function(method, path, body) {
    this.ws.send(JSON.stringify({
        method: method,
        path: path,
        body: body
    }));
};

Vue.component('svg-image', {
    data: function() {
        return {
            svgBody: ""
        }
    },
    props: ['src'],
    methods: {
        loadSrc: function() {
            loadAsync(this.src).then(svg => {
                // get rid of processing elements
                svg = svg.replace(/<\?.*>\s*/g, '');
                svg = svg.replace(/<!.*>\s*/g, '');
                // console.log(svg);

                this.svgBody = svg;
            });
        },
    },
    mounted: function() {
        this.loadSrc();
    },
    watch: {
        src: function(newVal, oldVal) {
            this.loadSrc();
        },
    },
    template: '<div class="svg-image" v-html="svgBody"></div>',
});

var days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
var months = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']

function formatDate(d) {
var hour = d.getHours();
var minute = d.getMinutes();
    var ampm = 'am';
    if(hour >= 12) {
        ampm = 'pm';
    }
    if(hour == 0) {
        hour = 12;
    } else if(hour > 12) {
        hour -= 12;
    }
    if(minute < 10) {
        minute = '0' + minute;
    }
    return days[d.getDay()] + ' ' + months[d.getMonth()] + ' ' + d.getDate() + numberSuffix(d.getDate()) + ' ' + hour + ':' + minute + ampm;
}

function formatTime(d) {
    var hour = d.getHours();
    var ampm = 'am';
    if(hour >= 12) {
        ampm = 'pm';
    }
    if(hour == 0) {
        hour = 12;
    } else if (hour > 12) {
        hour -= 12;
    }

    return '' + hour + ampm;
}

function numberSuffix(num) {
    // num = num << 0;

    var units = num % 10;
    var tens = Math.floor(num / 10) % 10;
    if (units == 1 && tens != 1) return 'st';
    if (units == 2 && tens != 1) return 'nd';
    if (units == 3 && tens != 1) return 'rd';
    return 'th';
}

Vue.component('clock', {
    data: function() {
        return {
            time: new Date(),
            formattedTime: ''
        }
    },
    methods: {
        updateTime: function() {
            this.time = new Date();
            this.formattedTime = formatDate(this.time);
        }
    },
    mounted: function() {
        updater = () => {
            this.updateTime();
            var seconds = new Date().getSeconds();
            setTimeout(updater, 1000*(60 - seconds));
        }
        updater();
    }
});

function load(websocketUrl) {
    var app = new App(websocketUrl, '#template');
}
