import RealtimeFile from './RealtimeFile';

export let rtc = {};

var hubName;
var intervalMs;
let socket;
let files = {};
let intervalID;

rtc.hub = function(hub) {
    hubName = hub;
    return rtc;
}

rtc.interval = function(interval) {
    intervalMs = interval;
    return rtc;
}

rtc.onFileChange = function(fileName, base, fileChangeCallback) {
    if (files.hasOwnProperty(fileName)) return files[fileName];
    let fileListener;
    if (!socket) {
        if (hubName && intervalMs) {
            socket = new WebSocket("wss://api.syncpoint.xyz?hub=" + hubName);
            fileListener = new RealtimeFileListener(fileName, base, socket, fileChangeCallback);
            files[fileName] = fileListener;
            socket.onmessage = (event) => {
                let message = JSON.parse(event.data);
                if (message.endpoint === "FILE_UPDATE") {
                    if (files.hasOwnProperty(message.file))
                        files[message.file]._file.receiveMessage(message);
                }
            }
            socket.onopen = (event) => {
                console.log('websocket connected!');
                for (let [_, listener] of Object.entries(files)) {
                    listener._file.fetchRemoteCommits();
                }
                intervalID = setInterval(() => {
                    console.log('---interval triggered---')
                    for (let [_, listener] of Object.entries(files)) {
                        listener._file.changeResolver();
                    }
                }, intervalMs);
            }
            socket.onclose = (event) => {
                console.log('websocket closed');
                clearInterval(intervalID);
            }
        } else
            throw new Error('please set hub and interval before trigger onFileChange');
    } else {
        fileListener = new RealtimeFileListener(fileName, base, socket, fileChangeCallback);
        files[fileName] = fileListener;
        if (this.socket.readyState === this.socket.OPEN)
            fileListener._file.fetchRemoteCommits();
    }
    return fileListener;
}

rtc.close = function() {
    socket.close();
    socket = null;
}

class RealtimeFileListener {
    constructor(fileName, base, socket, fileChangeCallback) {
        this._file = new RealtimeFile(fileName, base, socket, fileChangeCallback);
    }

    // tmp
    localOpChange(localOp) {
        this._file.handleLocalChange(localOp);
    }

    unsubscribe() {
        delete files[this._file.fileName];
    }
}