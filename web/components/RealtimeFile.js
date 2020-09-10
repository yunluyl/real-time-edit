import { v4 as uuidv4 } from 'uuid';
import { type as jot } from 'ot-json0';

export default class RealtimeFile {
    constructor(fileName, base, sockets, fileChangeCallback) {
        this.fileName = fileName;
        this.sockets = sockets;
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
            let committedFileChanged = false;
            let displayFileState;
            let displayFileStatePending = false;
            if (this.outstandingOp != null && !this.outstandingOpSuccess) {
                if (this.localOpBuffer)
                    this.localOpBuffer = jot.compose(this.outstandingOp, this.localOpBuffer);
                else this.localOpBuffer = this.outstandingOp;
                this.outstandingOp = null;
            }
            if (this.localOpBuffer != null && this.remoteOpBuffer != null) {
                console.log('local op before rebase');
                console.log(this.localOpBuffer);
                this.localOpBuffer = jot.transform(this.localOpBuffer, this.remoteOpBuffer, 'right');
                console.log('local op after rebase');
                if (this.localOpBuffer) console.log(this.localOpBuffer);
                else console.log('null');
            }
            if (this.outstandingOpSuccess) {
                console.log('commit self op: ' + (this.committedOpIndex + 1) + ' - ' + this.remoteIndexBuffer);
                console.log('self op');
                console.log(this.outstandingOp);
                console.log('committed file before self op apply');
                console.log(JSON.stringify(this.committedFile));
                this.committedFile = jot.apply(jot.create(this.committedFile), this.outstandingOp);
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
                console.log(this.remoteOpBuffer);
                console.log('committed file before apply remote');
                console.log(JSON.stringify(this.committedFile));
                this.committedFile = jot.apply(jot.create(this.committedFile), this.remoteOpBuffer);
                console.log('committed file after apply remote');
                console.log(JSON.stringify(this.committedFile));
                this.committedOpIndex = this.remoteIndexBuffer;
                this.remoteOpBuffer = null;
                committedFileChanged = true;
            }
            if (this.localOpBuffer) {
                console.log('send local op with index ' + (this.committedOpIndex + 1));
                console.log(this.localOpBuffer);
                console.log('committed file state before local apply');
                console.log(JSON.stringify(this.committedFile));
                displayFileState = jot.apply(jot.create(this.committedFile), this.localOpBuffer);
                console.log('display file state after local apply');
                console.log(displayFileState);
                console.log('committed file state after local apply');
                console.log(JSON.stringify(this.committedFile));
                displayFileStatePending = true;
                this.sendLocalOpBuffer();
            } else if (committedFileChanged) {
                displayFileState = jot.create(this.committedFile);
                displayFileStatePending = true;
            }
            if (displayFileStatePending) this.fileChangeCallback(displayFileState);
        }
    }

    handleRemoteCommits(message) {
        console.log('received remote op - length: ' + message.operations.length);
        let remoteOp = this.mergeRemoteOperations(message.index, message.operations);
        if (remoteOp != null) {
            if (this.remoteOpBuffer) this.remoteOpBuffer = jot.compose(this.remoteOpBuffer, remoteOp);
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
                if (op) op = jot.compose(op, JSON.parse(operations[i]));
                else op = JSON.parse(operations[i]);
            }
        }
        return op;
    }

    sendMessage(message) {
        if (this.outstandingMessageUid !== "") {
            console.error('only one outstanding message can be sent at a time');
            return;
        }
        if (this.sockets[0].readyState !== this.sockets[0].OPEN) {
            console.error('try to send message when socket is not open');
            return;
        }
        this.outstandingMessageUid = message.uid;
        this.sockets[0].send(JSON.stringify(message));
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
            operations: [JSON.stringify(this.localOpBuffer)],
        });
        this.outstandingOp = JSON.parse(JSON.stringify(this.localOpBuffer));
        this.localOpBuffer = null;
    }

    handleLocalChange(localOp) {
        if (localOp) {
            if (this.localOpBuffer) this.localOpBuffer = jot.compose(this.localOpBuffer, localOp);
            else this.localOpBuffer = localOp;
        }
    }
}