import { v4 as uuidv4 } from 'uuid';
import jot from 'jot';

export default class RealtimeFile {
    constructor(fileName, base, socket, fileChangeCallback) {
        this.fileName = fileName;
        this.socket = socket;
        this.fileChangeCallback = fileChangeCallback;
        this.committedFile = JSON.parse(JSON.stringify(base));
        this.committedOpIndex = -1;
        this.outstandingMessageUid = "";
        this.outstandingOp = null;
        this.outstandingOpSuccess = false;
        this.localOpBuffer = null;
        this.remoteOpBuffer = null;
        this.remoteIndexBuffer = -1;
    }

    fetchRemoteCommits() {
        this.sendMessage({
            uid: uuidv4(),
            endpoint: 'FILE_UPDATE',
            file: this.fileName,
            index: this.committedOpIndex,
            operations: [],
        });
    }

    changeResolver() {
        if (this.outstandingMessageUid === "") {
            console.log('---file change resolver triggered---');
            let committedFileChanged = false;
            let displayFileState;
            let displayFileStatePending = false;
            if (this.outstandingOp != null && !this.outstandingOpSuccess) {
                if (this.localOpBuffer)
                    this.localOpBuffer = this.outstandingOp.compose(this.localOpBuffer);
                else this.localOpBuffer = this.outstandingOp;
                this.outstandingOp = null;
            }
            if (this.localOpBuffer != null && this.remoteOpBuffer != null) {
                console.log('local op before rebase');
                console.log(this.localOpBuffer.serialize());
                this.localOpBuffer = this.localOpBuffer.rebase(this.remoteOpBuffer);
                console.log('local op after rebase');
                if (this.localOpBuffer) console.log(this.localOpBuffer.serialize());
                else console.log('null');
            }
            if (this.outstandingOpSuccess) {
                console.log('commit self op: ' + (this.committedOpIndex + 1) + ' - ' + this.remoteIndexBuffer);
                console.log('self op');
                console.log(this.outstandingOp.serialize());
                console.log('committed file before self op apply');
                console.log(JSON.stringify(this.committedFile));
                this.committedFile = JSON.parse(JSON.stringify(this.outstandingOp.apply(this.committedFile)));
                console.log('committed file after self op apply');
                console.log(JSON.stringify(this.committedFile));
                this.committedOpIndex = this.remoteIndexBuffer;
                this.outstandingOp = null;
                this.outstandingOpSuccess = false;
                committedFileChanged = true;
            }
            if (this.remoteOpBuffer) {
                console.log('commit remote op: ' + (this.committedOpIndex + 1) + ' - ' + this.remoteIndexBuffer);
                console.log('remote op');
                console.log(this.remoteOpBuffer.serialize());
                console.log('committed file before apply remote');
                console.log(JSON.stringify(this.committedFile));
                this.committedFile = JSON.parse(JSON.stringify(this.remoteOpBuffer.apply(this.committedFile)));
                console.log('committed file after apply remote');
                console.log(JSON.stringify(this.committedFile));
                this.committedOpIndex = this.remoteIndexBuffer;
                this.remoteOpBuffer = null;
                committedFileChanged = true;
            }
            if (this.localOpBuffer) {
                console.log('send local op with index ' + (this.committedOpIndex + 1));
                console.log(this.localOpBuffer.serialize());
                displayFileState = JSON.parse(JSON.stringify(this.localOpBuffer.apply(this.committedFile)));
                displayFileStatePending = true;
                this.sendLocalOpBuffer();
            } else if (committedFileChanged) {
                displayFileState = JSON.parse(JSON.stringify(this.committedFile));
                displayFileStatePending = true;
            }
            if (displayFileStatePending) this.fileChangeCallback(displayFileState);
        }
    }

    handleRemoteCommits(message) {
        console.log('received remote op - length: ' + message.operations.length);
        let remoteOp = this.mergeRemoteOperations(message.index, message.operations);
        if (remoteOp != null) {
            if (this.remoteOpBuffer) this.remoteOpBuffer = this.remoteOpBuffer.compose(remoteOp);
            else this.remoteOpBuffer = remoteOp;
        }
        this.remoteIndexBuffer += message.operations.length - this.remoteIndexBuffer + message.index - 1;
    }

    handleSelfCommits(message) {
        console.log('received self op - length: ' + message.operations.length);
        this.outstandingOpSuccess = true;
        this.remoteIndexBuffer += message.operations.length - this.remoteIndexBuffer + message.index - 1;
    }

    mergeRemoteOperations(remoteIndex, operations) {
        let op = null;
        if (operations) {
            let start = this.remoteIndexBuffer - remoteIndex + 1;
            if (start < 0) return null;
            for (let i = start; i < operations.length; i++) {
                if (op) op = op.compose(jot.deserialize(operations[i]));
                else op = jot.deserialize(operations[i]);
            }
        }
        return op;
    }

    sendMessage(message) {
        if (this.outstandingMessageUid !== "") {
            console.error('only one outstanding message can be sent at a time');
            return;
        }
        if (this.socket.readyState !== this.socket.OPEN) {
            console.error('try to send message when socket is not open');
            return;
        }
        this.outstandingMessageUid = message.uid;
        this.socket.send(JSON.stringify(message));
        console.log('---sent message---');
    }

    receiveMessage(message) {
        console.log('---received message---');
        if (message.file !== this.fileName) {
            console.error('received file name: ' + message.file + ' does not match the file: ' + this.fileName);
            return;
        }
        if (message.uid === this.outstandingMessageUid) {
            console.log('outstanding message cleared');
            this.outstandingMessageUid = "";
            message.resp = true;
        }
        console.log(message.status);
        console.log('message index: ' + message.index);
        if (message.index - this.remoteIndexBuffer > 1) {
            console.error(
                'index mismatch, remote index: '
                + message.index
                + ' local index: '
                + this.remoteIndexBuffer
            );
        }
        if (message.status === "OP_COMMITTED") {
            if (message.index - this.remoteIndexBuffer !== 1) {
                console.error(
                    'OP can only be committed in continuous sequence remote index: '
                    + message.index
                    + ' local index: '
                    + this.remoteIndexBuffer
                );
                console.error(message);
            }
            else {
                if (message.resp) this.handleSelfCommits(message);
                else this.handleRemoteCommits(message);
            }
        } else if (message.status === "OP_TOO_OLD")
            this.handleRemoteCommits(message);
        else {
            console.error('wrong file update return status: ' + message.status);
            console.error(message);
        }
    }

    sendLocalOpBuffer() {
        this.sendMessage({
            uid: uuidv4(),
            endpoint: 'FILE_UPDATE',
            file: this.fileName,
            index: this.committedOpIndex + 1,
            operations: [this.localOpBuffer.serialize()],
        });
        this.outstandingOp = jot.deserialize(this.localOpBuffer.serialize());
        this.localOpBuffer = null;
    }

    handleLocalChange(localOp) {
        if (localOp) {
            if (this.localOpBuffer) this.localOpBuffer = this.localOpBuffer.compose(localOp);
            else this.localOpBuffer = localOp;
        }
    }
}