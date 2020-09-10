import RealtimeFile from './RealtimeFile';

export let rtc = {};

let hubName;
let intervalMs;
let sockets = [null];
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
    let fileListener = new FileListener(fileName, base, sockets, fileChangeCallback);
    onFileChangeBase(fileName, fileListener);
    return fileListener;
}

rtc.onNotebookChange = function(fileName, base, fileChangeCallback) {
    let notebookListener = new NotebookListener(fileName, base, sockets, fileChangeCallback);
    onFileChangeBase(fileName, notebookListener);
    return notebookListener;
}

function onFileChangeBase(fileName, fileListener) {
    if (files.hasOwnProperty(fileName)) return files[fileName];
    if (!sockets[0]) {
        if (hubName && intervalMs) {
            sockets[0] = new WebSocket("wss://api.syncpoint.xyz?hub=" + hubName);
            files[fileName] = fileListener;
            sockets[0].onmessage = (event) => {
                let message = JSON.parse(event.data);
                if (message.endpoint === "FILE_UPDATE") {
                    if (files.hasOwnProperty(message.file))
                        files[message.file]._file.receiveMessage(message);
                }
            }
            sockets[0].onopen = (event) => {
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
            sockets[0].onclose = (event) => {
                console.log('websocket closed');
                clearInterval(intervalID);
            }
        } else
            throw new Error('please set hub and interval before trigger onFileChange');
    } else {
        files[fileName] = fileListener;
        if (this.sockets[0].readyState === this.sockets[0].OPEN)
            fileListener._file.fetchRemoteCommits();
    }
}

rtc.close = function() {
    if (sockets[0]) sockets[0].close();
    sockets[0] = null;
}

class FileListenerBase {
    constructor(fileName, base, sockets, fileChangeCallback) {
        this._file = new RealtimeFile(fileName, base, sockets, fileChangeCallback);
    }

    unsubscribe() {
        delete files[this._file.fileName];
    }
}

class FileListener extends FileListenerBase {
    constructor(fileName, base, sockets, fileChangeCallback) {
        super(fileName, base, sockets, fileChangeCallback);
    }

    insertText(path, text) {
        this._file.handleLocalChange([{p: path, si: text}]);
    }

    removeText(path, text) {
        this._file.handleLocalChange([{p: path, sd: text}]);
    }
}

class NotebookListener extends FileListenerBase {
    constructor(fileName, base, sockets, fileChangeCallback) {
        super(fileName, base, sockets, fileChangeCallback)
    }

    insertText(cell, pos, text) {
        this._file.handleLocalChange([{p: ['cells', cell, 'source', pos], si: text}]);
    }

    removeText(cell, pos, text) {
        this._file.handleLocalChange([{p: ['cells', cell, 'source', pos], sd: text}]);
    }

    insertCell(index, newCell) {
        this._file.handleLocalChange([{p: ['cells', index], li: JSON.parse(JSON.stringify(newCell))}]);
    }

    removeCell(index, oldCell) {
        this._file.handleLocalChange([{p: ['cells', index], ld: JSON.parse(JSON.stringify(oldCell))}]);
    }

    moveCell(oldIndex, newIndex) {
        this._file.handleLocalChange([{p: ['cells', oldIndex], lm: newIndex}]);
    }
}