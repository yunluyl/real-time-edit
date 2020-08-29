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
    this.opIndex = -1;
    this.bufferOp = null;
    this.outstandingOp = null;
    this.outstandingOpUid = "";
    this.numOfCells = 5;
    for (let i=0; i<this.numOfCells; i++) {
      this.state.file.cells.push({
          source: []
      });
      this.committedFile.cells.push({
          source: []
      });
    }
  }

  componentDidMount() {
    this.socket = new WebSocket("wss://localhost/ws?hub=aaa");
    this.socket.onopen = function (event) {
        console.log("websocket connected!");
        let message = {
            uid: uuidv4(),
            endpoint: 'FILE_UPDATE',
            index: -1,
            operations: [],
        };
        this.socket.send(JSON.stringify(message));
    }.bind(this);
    this.socket.onmessage = function (event) {
        let data = JSON.parse(event.data);
        if (data.endpoint === "FILE_UPDATE") {
            if (data.status === "OP_COMMITTED" || data.status === "OP_TOO_OLD") {
                if (data.index - this.opIndex > 1)
                    console.error('index mismatch, remote index: ' + data.index + 'local index: ' + this.opIndex);
                else if (data.index - this.opIndex === 1) {
                    console.log(data.status);
                    let remoteOp = null;
                    if (data.status === "OP_COMMITTED" && data.uid === this.outstandingOpUid) {
                        this.outstandingOp = null;
                    }
                    for (let i = 0; i < data.operations.length; i++) {
                        if (remoteOp)
                            remoteOp = remoteOp.compose(jot.deserialize(data.operations[i]));
                        else
                            remoteOp = jot.deserialize(data.operations[i]);
                    }
                    let allChanges = null;
                    let localOp = null;
                    if (this.outstandingOp) {
                        this.outstandingOp = this.outstandingOp.rebase(remoteOp);
                        if (this.outstandingOp) allChanges = remoteOp.compose(this.outstandingOp);
                        else allChanges = remoteOp;
                        if (this.bufferOp) {
                            this.bufferOp = this.bufferOp.rebase(allChanges);
                        }
                        localOp = this.outstandingOp;
                        if (localOp == null) localOp = this.bufferOp;
                        else if (this.bufferOp) localOp = localOp.compose(this.bufferOp);
                    } else if (this.bufferOp) {
                        if (data.status === "OP_COMMITTED" && data.uid === this.outstandingOpUid) {
                            localOp = this.bufferOp;
                        } else {
                            this.bufferOp = this.bufferOp.rebase(remoteOp);
                            if (this.bufferOp) localOp = this.bufferOp;
                        }
                    }
                    if (remoteOp) this.committedFile = remoteOp.apply(this.committedFile);
                    this.opIndex += data.operations.length;
                    if (localOp) {
                        this.state.file = localOp.apply(this.committedFile);
                        this.sendCommitOpMsg(localOp);
                    } else
                        this.state.file = JSON.parse(JSON.stringify(this.committedFile));
                    this.setState(this.state);
                }
            } else
                console.error('wrong file update return status: ' + data.status);
        }
    }.bind(this)
  }

  componentWillUnmount() {
      this.socket.close();
  }

    sendCommitOpMsg(op) {
      let message = {
          uid: uuidv4(),
          endpoint: 'FILE_UPDATE',
          index: this.opIndex + 1,
          operations: [op.serialize()],
      };
      this.bufferOp = null;
      this.outstandingOp = op;
      this.outstandingOpUid = message.uid;
      this.socket.send(JSON.stringify(message));
  }

  textChange(index, e) {
      e.preventDefault();
      let source = e.target.value.split('\n');
      for (let i=0; i<source.length-1; i++) {
          source[i] += '\n';
      }
      let oldFile = JSON.parse(JSON.stringify(this.state.file));
      this.state.file.cells[index]['source'] = source;
      let op = jot.diff(oldFile, this.state.file);
      if (this.bufferOp) op = this.bufferOp.compose(op);
      if (this.outstandingOp != null || this.socket.readyState !== this.socket.OPEN) {
          this.bufferOp = op;
      }
      else {
          this.sendCommitOpMsg(op);
      }
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
                minRows='1'
                value={source.join('')} />
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