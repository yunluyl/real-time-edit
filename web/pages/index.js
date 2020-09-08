import React, { Component } from 'react';
import Router, { withRouter } from 'next/router';
import TextareaAutosize from 'react-textarea-autosize';
import jot from 'jot';
import styles from '../styles/Index.module.css';
import { rtc } from '../components/RealtimeClient';

class IndexPage extends Component {
    constructor(props) {
        super(props);
        this.hubName = Router.query.hub;
        this.fileName = "/home/jupyter/tutorils/test.ipynb"
        this.state = {
            file: {
                cells: [],
            }
        };
        this.numOfCells = 5;
        for (let i = 0; i < this.numOfCells; i++) {
            this.state.file.cells.push({
                source: ""
            });
        }
    }

    componentDidMount() {
        this.fileListener = rtc.hub(this.hubName).interval(500).onFileChange(
            this.fileName,
            this.state.file,
            (file) => {
            this.state.file = file;
            this.setState(this.state);
        });
    }

    componentWillUnmount() {
        this.fileListener.unsubscribe();
        rtc.close();
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
        this.fileListener.localOpChange(localOp);
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
        for (let i = 0; i < this.numOfCells; i++) {
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