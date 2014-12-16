package raft

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/shelmesky/raft/protobuf"
)

//------------------------------------------------------------------------------
//
// Typedefs
//
//------------------------------------------------------------------------------

// A log is a collection of log entries that are persisted to durable storage.
// 日志是日志记录的集合，并且有持久化的保证。
type Log struct {
	ApplyFunc   func(*LogEntry, Command) (interface{}, error)
	file        *os.File
	path        string
	entries     []*LogEntry
	commitIndex uint64
	mutex       sync.RWMutex
	startIndex  uint64 // the index before the first entry in the Log entries
	// startIndex是相对于第一条日志的记录数
	startTerm   uint64
	initialized bool
}

// The results of the applying a log entry.
type logResult struct {
	returnValue interface{}
	err         error
}

//------------------------------------------------------------------------------
//
// Constructor
//
//------------------------------------------------------------------------------

// Creates a new log.
func newLog() *Log {
	return &Log{
		entries: make([]*LogEntry, 0),
	}
}

//------------------------------------------------------------------------------
//
// Accessors
//
//------------------------------------------------------------------------------

//--------------------------------------
// Log Indices
//--------------------------------------

// 最后被提交的日志的index
// The last committed index in the log.
func (l *Log) CommitIndex() uint64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.commitIndex
}

// 当前日志的index
// The current index in the log.
func (l *Log) currentIndex() uint64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.internalCurrentIndex()
}

// 当前日志中没有被"锁定"的日志index
// The current index in the log without locking
func (l *Log) internalCurrentIndex() uint64 {
	if len(l.entries) == 0 {
		return l.startIndex
	}
	return l.entries[len(l.entries)-1].Index()
}

// 下一个日志的index
// The next index in the log.
func (l *Log) nextIndex() uint64 {
	return l.currentIndex() + 1
}

// 日志是否为空
// Determines if the log contains zero entries.
func (l *Log) isEmpty() bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return (len(l.entries) == 0) && (l.startIndex == 0)
}

// 日志中最后一个command的名称
// The name of the last command in the log.
func (l *Log) lastCommandName() string {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if len(l.entries) > 0 {
		if entry := l.entries[len(l.entries)-1]; entry != nil {
			return entry.CommandName()
		}
	}
	return ""
}

//--------------------------------------
// Log Terms
//--------------------------------------

// The current term in the log.
func (l *Log) currentTerm() uint64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if len(l.entries) == 0 {
		return l.startTerm
	}
	return l.entries[len(l.entries)-1].Term()
}

//------------------------------------------------------------------------------
//
// Methods
//
//------------------------------------------------------------------------------

//--------------------------------------
// State
//--------------------------------------

// Opens the log file and reads existing entries. The log can remain open and
// continue to append entries to the end of the log.
// 打开log文件，并读取已存在的日志记录。
func (l *Log) open(path string) error {
	// Read all the entries from the log if one exists.
	var readBytes int64

	var err error
	debugln("log.open.open ", path)
	// open log file
	// 打开日志文件
	l.file, err = os.OpenFile(path, os.O_RDWR, 0600)
	l.path = path

	if err != nil {
		// if the log file does not exist before
		// we create the log file and set commitIndex to 0
		// 如果日志文件不存在，则创建一个并设置commitIndex为0
		if os.IsNotExist(err) {
			l.file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
			debugln("log.open.create ", path)
			if err == nil {
				l.initialized = true
			}
			return err
		}
		return err
	}
	debugln("log.open.exist ", path)

	// 读取日志文件并解码记录
	// Read the file and decode entries.
	for {
		// Instantiate log entry and decode into it.

		// 初始化一个空的日志
		entry, _ := newLogEntry(l, nil, 0, 0, nil)
		// 文件seek到0
		entry.Position, _ = l.file.Seek(0, os.SEEK_CUR)

		// 从文件中读取一条日志记录
		n, err := entry.Decode(l.file)
		if err != nil {
			// 读到文件尾
			if err == io.EOF {
				debugln("open.log.append: finish ")
				// 读取错误，将文件truncate
			} else {
				if err = os.Truncate(path, readBytes); err != nil {
					return fmt.Errorf("raft.Log: Unable to recover: %v", err)
				}
			}
			// 退出循环
			break
		}
		// 如果读取的日志记录的index大于日志开始的index
		if entry.Index() > l.startIndex {
			// Append entry.
			// 将日志添加到l.entries
			l.entries = append(l.entries, entry)
			// 获取日志中记录的command，并执行命令
			if entry.Index() <= l.commitIndex {
				command, err := newCommand(entry.CommandName(), entry.Command())
				if err != nil {
					// 如果命令信息出错，则继续循环
					continue
				}
				// 调用在Server.Start中绑定的ApplyFunc函数
				l.ApplyFunc(entry, command)
			}
			debugln("open.log.append log index ", entry.Index())
		}

		readBytes += int64(n)
	}
	debugln("open.log.recovery number of log ", len(l.entries))
	l.initialized = true
	return nil
}

// Closes the log file.
func (l *Log) close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	l.entries = make([]*LogEntry, 0)
}

// sync to disk
func (l *Log) sync() error {
	return l.file.Sync()
}

//--------------------------------------
// Entries
//--------------------------------------

// Creates a log entry associated with this log.
func (l *Log) createEntry(term uint64, command Command, e *ev) (*LogEntry, error) {
	return newLogEntry(l, e, l.nextIndex(), term, command)
}

// Retrieves an entry from the log. If the entry has been eliminated because
// of a snapshot then nil is returned.
func (l *Log) getEntry(index uint64) *LogEntry {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if index <= l.startIndex || index > (l.startIndex+uint64(len(l.entries))) {
		return nil
	}
	return l.entries[index-l.startIndex-1]
}

// Checks if the log contains a given index/term combination.
func (l *Log) containsEntry(index uint64, term uint64) bool {
	entry := l.getEntry(index)
	return (entry != nil && entry.Term() == term)
}

// Retrieves a list of entries after a given index as well as the term of the
// index provided. A nil list of entries is returned if the index no longer
// exists because a snapshot was made.
func (l *Log) getEntriesAfter(index uint64, maxLogEntriesPerRequest uint64) ([]*LogEntry, uint64) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	// Return nil if index is before the start of the log.
	if index < l.startIndex {
		traceln("log.entriesAfter.before: ", index, " ", l.startIndex)
		return nil, 0
	}

	// Return an error if the index doesn't exist.
	if index > (uint64(len(l.entries)) + l.startIndex) {
		panic(fmt.Sprintf("raft: Index is beyond end of log: %v %v", len(l.entries), index))
	}

	// If we're going from the beginning of the log then return the whole log.
	if index == l.startIndex {
		traceln("log.entriesAfter.beginning: ", index, " ", l.startIndex)
		return l.entries, l.startTerm
	}

	traceln("log.entriesAfter.partial: ", index, " ", l.entries[len(l.entries)-1].Index)

	entries := l.entries[index-l.startIndex:]
	length := len(entries)

	traceln("log.entriesAfter: startIndex:", l.startIndex, " length", len(l.entries))

	if uint64(length) < maxLogEntriesPerRequest {
		// Determine the term at the given entry and return a subslice.
		return entries, l.entries[index-1-l.startIndex].Term()
	} else {
		return entries[:maxLogEntriesPerRequest], l.entries[index-1-l.startIndex].Term()
	}
}

//--------------------------------------
// Commit
//--------------------------------------

// Retrieves the last index and term that has been committed to the log.
func (l *Log) commitInfo() (index uint64, term uint64) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	// If we don't have any committed entries then just return zeros.
	if l.commitIndex == 0 {
		return 0, 0
	}

	// No new commit log after snapshot
	if l.commitIndex == l.startIndex {
		return l.startIndex, l.startTerm
	}

	// Return the last index & term from the last committed entry.
	debugln("commitInfo.get.[", l.commitIndex, "/", l.startIndex, "]")
	entry := l.entries[l.commitIndex-1-l.startIndex]
	return entry.Index(), entry.Term()
}

// Retrieves the last index and term that has been appended to the log.
func (l *Log) lastInfo() (index uint64, term uint64) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	// If we don't have any entries then just return zeros.
	if len(l.entries) == 0 {
		return l.startIndex, l.startTerm
	}

	// Return the last index & term
	entry := l.entries[len(l.entries)-1]
	return entry.Index(), entry.Term()
}

// Updates the commit index
func (l *Log) updateCommitIndex(index uint64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if index > l.commitIndex {
		l.commitIndex = index
	}
	debugln("update.commit.index ", index)
}

// Updates the commit index and writes entries after that index to the stable storage.
func (l *Log) setCommitIndex(index uint64) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// this is not error any more after limited the number of sending entries
	// commit up to what we already have
	/*
		当commit的index大于当前节点所有的entries时，将最大的index设置为当前节点的
	*/
	if index > l.startIndex+uint64(len(l.entries)) {
		debugln("raft.Log: Commit index", index, "set back to ", len(l.entries))
		index = l.startIndex + uint64(len(l.entries))
	}

	// Do not allow previous indices to be committed again.

	// This could happens, since the guarantee is that the new leader has up-to-dated
	// log entries rather than has most up-to-dated committed index

	// For example, Leader 1 send log 80 to follower 2 and follower 3
	// follower 2 and follow 3 all got the new entries and reply
	// leader 1 committed entry 80 and send reply to follower 2 and follower3
	// follower 2 receive the new committed index and update committed index to 80
	// leader 1 fail to send the committed index to follower 3
	// follower 3 promote to leader (server 1 and server 2 will vote, since leader 3
	// has up-to-dated the entries)
	// when new leader 3 send heartbeat with committed index = 0 to follower 2,
	// follower 2 should reply success and let leader 3 update the committed index to 80

	/*
		不允许之前的一些index(复数)再次成为commited

		举例来说，leader 1发送log 80给follower2和follower3，
		follower2和follower3都获得了最新的entries并且返回给leader 1,
		leader 1提交entry 80并且发送响应给follower2和follower3,
		于是follower3收到了新的已经被commited的index并且更新commited index到80.
		但是leader 1发送commited index到follower3失败，
		follower3升级为leader(server 1和server 2会投票，因为leader 3拥有最新的entries)
		当新的leader 3携带commited index为0，然后发送心跳给follower2，
		follower3应该返回成功，并且让leader 3升级commited index到80
	*/

	// 如果需要commit的index小于当前节点的commitIndex，返回nil，表示成功
	if index < l.commitIndex {
		return nil
	}

	// Find all entries whose index is between the previous index and the current index.
	for i := l.commitIndex + 1; i <= index; i++ {
		entryIndex := i - 1 - l.startIndex
		entry := l.entries[entryIndex]

		// Update commit index.
		l.commitIndex = entry.Index()

		// Decode the command.
		command, err := newCommand(entry.CommandName(), entry.Command())
		if err != nil {
			return err
		}

		// Apply the changes to the state machine and store the error code.
		returnValue, err := l.ApplyFunc(entry, command)

		debugf("setCommitIndex.set.result index: %v, entries index: %v", i, entryIndex)
		if entry.event != nil {
			entry.event.returnValue = returnValue
			entry.event.c <- err
		}

		_, isJoinCommand := command.(JoinCommand)

		// we can only commit up to the most recent join command
		// if there is a join in this batch of commands.
		// after this commit, we need to recalculate the majority.
		if isJoinCommand {
			return nil
		}
	}
	return nil
}

// Set the commitIndex at the head of the log file to the current
// commit Index. This should be called after obtained a log lock
func (l *Log) flushCommitIndex() {
	l.file.Seek(0, os.SEEK_SET)
	fmt.Fprintf(l.file, "%8x\n", l.commitIndex)
	l.file.Seek(0, os.SEEK_END)
}

//--------------------------------------
// Truncation
//--------------------------------------

// Truncates the log to the given index and term. This only works if the log
// at the index has not been committed.
func (l *Log) truncate(index uint64, term uint64) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	debugln("log.truncate: ", index)

	// Do not allow committed entries to be truncated.
	if index < l.commitIndex {
		debugln("log.truncate.before")
		return fmt.Errorf("raft.Log: Index is already committed (%v): (IDX=%v, TERM=%v)", l.commitIndex, index, term)
	}

	// Do not truncate past end of entries.
	if index > l.startIndex+uint64(len(l.entries)) {
		debugln("log.truncate.after")
		return fmt.Errorf("raft.Log: Entry index does not exist (MAX=%v): (IDX=%v, TERM=%v)", len(l.entries), index, term)
	}

	// If we're truncating everything then just clear the entries.
	if index == l.startIndex {
		debugln("log.truncate.clear")
		l.file.Truncate(0)
		l.file.Seek(0, os.SEEK_SET)

		// notify clients if this node is the previous leader
		for _, entry := range l.entries {
			if entry.event != nil {
				entry.event.c <- errors.New("command failed to be committed due to node failure")
			}
		}

		l.entries = []*LogEntry{}
	} else {
		// Do not truncate if the entry at index does not have the matching term.
		entry := l.entries[index-l.startIndex-1]
		if len(l.entries) > 0 && entry.Term() != term {
			debugln("log.truncate.termMismatch")
			return fmt.Errorf("raft.Log: Entry at index does not have matching term (%v): (IDX=%v, TERM=%v)", entry.Term(), index, term)
		}

		// Otherwise truncate up to the desired entry.
		if index < l.startIndex+uint64(len(l.entries)) {
			debugln("log.truncate.finish")
			position := l.entries[index-l.startIndex].Position
			l.file.Truncate(position)
			l.file.Seek(position, os.SEEK_SET)

			// notify clients if this node is the previous leader
			for i := index - l.startIndex; i < uint64(len(l.entries)); i++ {
				entry := l.entries[i]
				if entry.event != nil {
					entry.event.c <- errors.New("command failed to be committed due to node failure")
				}
			}

			l.entries = l.entries[0 : index-l.startIndex]
		}
	}

	return nil
}

//--------------------------------------
// Append
//--------------------------------------

// Appends a series of entries to the log.
func (l *Log) appendEntries(entries []*protobuf.LogEntry) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	startPosition, _ := l.file.Seek(0, os.SEEK_CUR)

	w := bufio.NewWriter(l.file)

	var size int64
	var err error
	// Append each entry but exit if we hit an error.
	for i := range entries {
		logEntry := &LogEntry{
			log:      l,
			Position: startPosition,
			pb:       entries[i],
		}

		if size, err = l.writeEntry(logEntry, w); err != nil {
			return err
		}

		startPosition += size
	}
	w.Flush()
	err = l.sync()

	if err != nil {
		panic(err)
	}

	return nil
}

// Writes a single log entry to the end of the log.
func (l *Log) appendEntry(entry *LogEntry) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.file == nil {
		return errors.New("raft.Log: Log is not open")
	}

	// Make sure the term and index are greater than the previous.
	if len(l.entries) > 0 {
		lastEntry := l.entries[len(l.entries)-1]
		if entry.Term() < lastEntry.Term() {
			return fmt.Errorf("raft.Log: Cannot append entry with earlier term (%x:%x <= %x:%x)", entry.Term(), entry.Index(), lastEntry.Term(), lastEntry.Index())
		} else if entry.Term() == lastEntry.Term() && entry.Index() <= lastEntry.Index() {
			return fmt.Errorf("raft.Log: Cannot append entry with earlier index in the same term (%x:%x <= %x:%x)", entry.Term(), entry.Index(), lastEntry.Term(), lastEntry.Index())
		}
	}

	position, _ := l.file.Seek(0, os.SEEK_CUR)

	entry.Position = position

	// Write to storage.
	if _, err := entry.Encode(l.file); err != nil {
		return err
	}

	// Append to entries list if stored on disk.
	l.entries = append(l.entries, entry)

	return nil
}

// appendEntry with Buffered io
func (l *Log) writeEntry(entry *LogEntry, w io.Writer) (int64, error) {
	if l.file == nil {
		return -1, errors.New("raft.Log: Log is not open")
	}

	// Make sure the term and index are greater than the previous.
	if len(l.entries) > 0 {
		lastEntry := l.entries[len(l.entries)-1]
		if entry.Term() < lastEntry.Term() {
			return -1, fmt.Errorf("raft.Log: Cannot append entry with earlier term (%x:%x <= %x:%x)", entry.Term(), entry.Index(), lastEntry.Term(), lastEntry.Index())
		} else if entry.Term() == lastEntry.Term() && entry.Index() <= lastEntry.Index() {
			return -1, fmt.Errorf("raft.Log: Cannot append entry with earlier index in the same term (%x:%x <= %x:%x)", entry.Term(), entry.Index(), lastEntry.Term(), lastEntry.Index())
		}
	}

	// Write to storage.
	size, err := entry.Encode(w)
	if err != nil {
		return -1, err
	}

	// Append to entries list if stored on disk.
	l.entries = append(l.entries, entry)

	return int64(size), nil
}

//--------------------------------------
// Log compaction
//--------------------------------------

// compact the log before index (including index)
func (l *Log) compact(index uint64, term uint64) error {
	var entries []*LogEntry

	l.mutex.Lock()
	defer l.mutex.Unlock()

	if index == 0 {
		return nil
	}
	// nothing to compaction
	// the index may be greater than the current index if
	// we just recovery from on snapshot
	if index >= l.internalCurrentIndex() {
		entries = make([]*LogEntry, 0)
	} else {
		// get all log entries after index
		entries = l.entries[index-l.startIndex:]
	}

	// create a new log file and add all the entries
	new_file_path := l.path + ".new"
	file, err := os.OpenFile(new_file_path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		position, _ := l.file.Seek(0, os.SEEK_CUR)
		entry.Position = position

		if _, err = entry.Encode(file); err != nil {
			file.Close()
			os.Remove(new_file_path)
			return err
		}
	}
	file.Sync()

	old_file := l.file

	// rename the new log file
	err = os.Rename(new_file_path, l.path)
	if err != nil {
		file.Close()
		os.Remove(new_file_path)
		return err
	}
	l.file = file

	// close the old log file
	old_file.Close()

	// compaction the in memory log
	l.entries = entries
	l.startIndex = index
	l.startTerm = term
	return nil
}
