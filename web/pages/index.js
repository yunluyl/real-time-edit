import React, {Component} from 'react';
import Router, { withRouter } from 'next/router';
import TextareaAutosize from 'react-textarea-autosize';
import { v4 as uuidv4 } from 'uuid';
import jot from 'jot';
import styles from '../styles/Index.module.css'

class IndexPage extends Component {
  constructor(props) {
    super(props);
    this.state = {
      file: {
        cells: [],
      }
    };
    this.committedFile = {
      cells: [],
    };
    this.committedOpIndex = -1;
    this.outstandingMessageUid = "";
    this.outstandingOp = null;
    this.outstandingOpSuccess = false;
    this.localOpBuffer = null;
    this.remoteOpBuffer = null;
    this.remoteIndexBuffer = -1;
    this.numOfCells = 5;
    for (let i=0; i<this.numOfCells; i++) {
      this.state.file.cells.push({
        source: ""
      });
      this.committedFile.cells.push({
        source: ""
      });
    }
  }

  componentDidMount() {
    this.socket = new WebSocket("wss://api.syncpoint.xyz?hub=aaa");
    this.socket.onopen = function (event) {
      console.log("websocket connected!");
      this.sendMessage({
        uid: uuidv4(),
        endpoint: 'FILE_UPDATE',
        index: -1,
        operations: [],
      });
    }.bind(this);
    this.socket.onmessage = function (event) {
      let message = this.receiveMessage(event);
      if (message.endpoint === "FILE_UPDATE") {
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
          if (message.index - this.remoteIndexBuffer !== 1)
            console.error(
              'OP can only be committed in continuous sequence remote index: '
              + message.index
              + ' local index: '
              + this.remoteIndexBuffer
            );
          else {
            if (message.resp) this.handleSelfCommits(message);
            else this.handleRemoteCommits(message);
          }
        } else if (message.status === "OP_TOO_OLD")
          this.handleRemoteCommits(message);
        else
          console.error('wrong file update return status: ' + message.status);
      }
    }.bind(this);
    this.intervalID = setInterval(() => {
      if (this.outstandingMessageUid === "") {
        console.log('---interval triggered---');
        let committedFileChanged = false;
        let setStatePending = false;
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
          this.state.file = JSON.parse(JSON.stringify(this.localOpBuffer.apply(this.committedFile)));
          this.sendLocalOpBuffer();
          setStatePending = true;
        } else if (committedFileChanged) {
          this.state.file = JSON.parse(JSON.stringify(this.committedFile));
          setStatePending = true;
        }
        if (setStatePending) this.setState(this.state);
      }
      }, 500);
  }

  componentWillUnmount() {
    this.socket.close();
    clearInterval(this.intervalID);
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
    if (this.outstandingMessageUid !== "")
      console.error('only one outstanding message can be sent at a time');
    this.outstandingMessageUid = message.uid;
    this.socket.send(JSON.stringify(message));
    console.log('---sent message---');
  }

  receiveMessage(event) {
    console.log('---received message---');
    let message = JSON.parse(event.data);
    if (message.uid === this.outstandingMessageUid) {
      console.log('outstanding message cleared');
      this.outstandingMessageUid = "";
      message.resp = true;
    }
    return message;
  }

  sendLocalOpBuffer() {
    this.sendMessage({
      uid: uuidv4(),
      endpoint: 'FILE_UPDATE',
      index: this.committedOpIndex + 1,
      operations: [this.localOpBuffer.serialize()],
    });
    this.outstandingOp = jot.deserialize(this.localOpBuffer.serialize());
    this.localOpBuffer = null;
  }

  textChange(index, e) {
    e.preventDefault();
    if (e.target.selectionStart !== e.target.selectionEnd)
      console.error('cursor start end end not equal on text change event');

    let start;
    let endOld;
    let endNew;
    let quickMatch = false;
    const quickMatchLength = 250;
    const lenDiff = Math.abs(this.state.file.cells[index]['source'].length - e.target.value.length);
    if (this.state.file.cells[index]['source'].length > e.target.value.length) {
      if (
        this.state.file.cells[index]['source'].substring(
          e.target.selectionStart - quickMatchLength,
          e.target.selectionStart) ===
        e.target.value.substring(
          e.target.selectionStart - quickMatchLength,
          e.target.selectionStart) &&
        this.state.file.cells[index]['source'].substring(
          e.target.selectionStart + lenDiff,
          e.target.selectionStart + lenDiff + quickMatchLength) ===
        e.target.value.substring(e.target.selectionStart, e.target.selectionStart + quickMatchLength)
      ) {
        start = e.target.selectionStart;
        endOld = e.target.selectionStart + lenDiff;
        endNew = e.target.selectionStart;
        quickMatch = true;
      }
    } else if (this.state.file.cells[index]['source'].length < e.target.value.length) {
      if (this.state.file.cells[index]['source'].substring(
        e.target.selectionStart - lenDiff - quickMatchLength,
        e.target.selectionStart - lenDiff) ===
        e.target.value.substring(
          e.target.selectionStart - lenDiff - quickMatchLength,
          e.target.selectionStart - lenDiff) &&
        this.state.file.cells[index]['source'].substring(
          e.target.selectionStart - lenDiff,
          e.target.selectionStart - lenDiff + quickMatchLength) ===
        e.target.value.substring(e.target.selectionStart, e.target.selectionStart + quickMatchLength)
      ) {
        start = e.target.selectionStart - lenDiff;
        endOld = e.target.selectionStart - lenDiff;
        endNew = e.target.selectionStart;
        quickMatch = true;
      }
    }

    if (!quickMatch) {
      for (start = 0;
           start < Math.min(this.state.file.cells[index]['source'].length, e.target.value.length);
           start++) {
        if (this.state.file.cells[index]['source'].charAt(start) !== e.target.value.charAt(start))
          break;
      }
      endOld = this.state.file.cells[index]['source'].length - 1;
      endNew = e.target.value.length - 1;
      while (endOld >= start && endNew >= start) {
        if (this.state.file.cells[index]['source'].charAt(endOld) !== e.target.value.charAt(endNew))
          break;
        endOld--;
        endNew--;
      }
      endOld++;
      endNew++;
    }

    let localOp = null;
    if (start !== endOld || start !== endNew) {
      localOp = new jot.APPLY('cells',
        new jot.ATINDEX(index,
          new jot.APPLY('source',
            new jot.SPLICE(start, endOld - start,
              e.target.value.substring(start, endNew)))));
    }
    if (localOp) {
      if (this.localOpBuffer) this.localOpBuffer = this.localOpBuffer.compose(localOp);
      else this.localOpBuffer = localOp;
    }
    this.state.file.cells[index]['source'] = e.target.value;
    //if (localOp != null &&
    //  this.outstandingMessageUid === "" &&
    //  this.socket.readyState === this.socket.OPEN) {
    //  this.sendCommitOpMsg(localOp);
    //}
    this.setState(this.state);
  }

  generateCells() {
    let cells = [];
    for (let i=0; i<this.numOfCells; i++) {
      let source = this.state.file.cells[i]['source'];
      cells.push(
        <div key={i}>
          <TextareaAutosize
            key={i}
            className={styles.text_area}
            onChange={this.textChange.bind(this, i)}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck="false"
            minRows='1'
            value={source} />
        </div>
      )
    }
    return cells
  }

  render() {
    return (
      <div>
        {this.generateCells()}
      </div>
    );
  }
}

export default withRouter(IndexPage);