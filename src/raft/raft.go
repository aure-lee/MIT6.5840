package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	//	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labrpc"
)

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 3D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 3D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

type Role string

const (
	Follower  Role = "Follower"
	Candidate Role = "Candidate"
	Leader    Role = "Leader"
)

// the inf of log entry
type LogEntry struct {
	Term    int
	Command interface{}
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()
	applyCh   chan ApplyMsg

	// Your data here (3A, 3B, 3C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// Persistent state on all servers
	currentTerm int
	votedFor    int // when currentTerm change, votedFor change
	logs        []LogEntry

	// Volatile state on all servers
	commitIndex int
	lastApplied int

	// Volatile state on leaders
	nextIndex  []int
	matchIndex []int

	role       Role
	heartChan  chan struct{}
	lastUpdate time.Time
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term := rf.currentTerm
	isleader := rf.role == Leader

	return term, isleader
}

// must be used in mutex
func (rf *Raft) GetLastLogIndex() int {
	return len(rf.logs) - 1
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (3C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (3D).

}

// must be used in mutex
func (rf *Raft) isLogUpToDate(prevLogTerm, prevLogIndex int) bool {
	lastLogIndex := rf.GetLastLogIndex()
	if prevLogTerm == rf.logs[lastLogIndex].Term {
		return prevLogIndex >= lastLogIndex
	}

	return prevLogTerm > rf.logs[lastLogIndex].Term
}

type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

// RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	DPrintf("{Server %v, %v, Term %v, Last log index %v} Receive RequestVote from Server %v for term %v and vote for %v\n",
		rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), args.CandidateId, args.Term, rf.votedFor)

	// 1. term < currentTerm: reply false
	// 2. term == currentTerm and have voted: reply false
	// 3. term > currentTerm: convert to follower, reset rf
	// 4. term == currentTerm and not vote: compare log
	// 5. compare lastLogIndex and lastLogTerm

	if args.Term < rf.currentTerm {
		reply.Term, reply.VoteGranted = rf.currentTerm, false
		return
	} else if args.Term == rf.currentTerm && rf.votedFor != -1 && rf.votedFor != args.CandidateId {
		reply.Term, reply.VoteGranted = rf.currentTerm, false
		return
	}

	if args.Term > rf.currentTerm {
		DPrintf("{Server %v, %v, Term %v, Last log index %v} Receive term %v > current term %v\n",
			rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), args.Term, rf.currentTerm)
		rf.currentTerm = args.Term
		rf.convertRole(Follower)
	}

	// 如果符合条件的candidate的日志不是最新的，不投票给他，同时重置votedFor
	if !rf.isLogUpToDate(args.LastLogTerm, args.LastLogIndex) {
		reply.Term, reply.VoteGranted = rf.currentTerm, false
		rf.votedFor = -1
		return
	}

	reply.Term, reply.VoteGranted = rf.currentTerm, true

	rf.votedFor = args.CandidateId
	rf.lastUpdate = time.Now()
	DPrintf("{Server %v, %v, Term %v, Last log index %v} Vote for Server %v\n",
		rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), rf.votedFor)
}

func (rf *Raft) genRequestVoteArgs() RequestVoteArgs {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.GetLastLogIndex(),
		LastLogTerm:  rf.logs[rf.GetLastLogIndex()].Term,
	}
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) startElection() {
	rf.mu.Lock()
	rf.convertRole(Candidate)
	rf.currentTerm++
	rf.votedFor = rf.me
	rf.mu.Unlock()

	// DPrintf("{Server %v, %v, Term %v, Last log index %v} Start Election\n",
	// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex())

	args := rf.genRequestVoteArgs()

	// send RequestVote RPCs to all other servers
	voteCount := 1

	for i := range rf.peers {
		if i != rf.me {
			// 将i作为闭包传递给goroutine，因为直接在goroutine中调用i，可能会调用已经修改过的i
			go func(server int) {
				reply := RequestVoteReply{}
				if rf.sendRequestVote(server, &args, &reply) {
					DPrintf("{Server %v, %v, Term %v, Last log index %v} Send RequestVoteArgs to server %v\n",
						rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), server)
					rf.mu.Lock()
					defer rf.mu.Unlock()
					if rf.role == Candidate && reply.VoteGranted {
						voteCount++
						// if votes received from majority of servers: become leader
						if voteCount > len(rf.peers)/2 {
							rf.convertRole(Leader)
							return
						}
					} else if reply.Term > rf.currentTerm {
						// DPrintf("{Server %v, %v, Term %v, Last log index %v} Discovers a new term %v, convert to follower\n",
						// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), reply.Term)
						rf.currentTerm = reply.Term
						rf.convertRole(Follower)
					}
				}
			}(i)
		}
	}

}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int // followers can redirect client
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int // leader's commitIndex
}

type AppendEntriesReply struct {
	Term    int
	Success bool // true if follower contained entry matching prevLogIndex and prevLogTerm
}

func (rf *Raft) matchIndexAndTerm(index, term int) bool {
	if index > rf.GetLastLogIndex() {
		return false
	}
	return index < 0 || rf.logs[index].Term == term
}

func (rf *Raft) applyLogs() {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	for rf.lastApplied < rf.commitIndex {
		i := rf.lastApplied + 1
		rf.applyCh <- ApplyMsg{
			CommandValid: true,
			Command:      rf.logs[i].Command,
			CommandIndex: i,
		}
		rf.lastApplied = i
	}
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	DPrintf("{Server %v, %v, Term %v, Last log index %v} Receive Heartbeat from server %v and term is %v\n",
		rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), args.LeaderId, args.Term)

	// if AppendEntries RPC received from now leader: convert to follower
	if args.Term < rf.currentTerm {
		reply.Term, reply.Success = rf.currentTerm, false
		return
	} else if args.Term >= rf.currentTerm {
		rf.convertRole(Follower)
		rf.currentTerm = args.Term
		rf.lastUpdate = time.Now()
	}

	// 如果有附加日志
	if !rf.matchIndexAndTerm(args.PrevLogIndex, args.PrevLogTerm) {
		reply.Term, reply.Success = rf.currentTerm, false
		return
	}

	rf.logs = rf.logs[:args.PrevLogIndex+1]
	if len(args.Entries) > 0 {
		rf.logs = append(rf.logs, args.Entries...)
	}

	// DPrintf("{Server %v, %v, Term %v, Last log index %v} Receive %v length logs from server %v\n",
	// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), len(args.Entries), args.LeaderId)

	if args.LeaderCommit > rf.commitIndex {
		if args.LeaderCommit < rf.GetLastLogIndex() {
			rf.commitIndex = args.LeaderCommit
		} else {
			rf.commitIndex = rf.GetLastLogIndex()
		}
		// commitIndex更新后需要应用新的条目到状态机中
		go rf.applyLogs()
	}

	reply.Term, reply.Success = args.Term, true
}

func (rf *Raft) genAppendEntries(server int) AppendEntriesArgs {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	args := AppendEntriesArgs{}
	args.Term = rf.currentTerm
	args.LeaderId = rf.me
	args.PrevLogIndex = rf.nextIndex[server] - 1
	if args.PrevLogIndex < 0 {
		args.PrevLogTerm = 0
	} else {
		args.PrevLogTerm = rf.logs[args.PrevLogIndex].Term
	}
	entries := rf.logs[rf.nextIndex[server]:]
	args.Entries = make([]LogEntry, len(entries))
	copy(args.Entries, entries)
	args.LeaderCommit = rf.commitIndex

	return args
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	DPrintf("{Server %v, %v, Term %v, Last log index %v} Send Heartbeat to server %v\n",
		rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), server)
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

// if send AppendEntries success, handle the reply
func (rf *Raft) handleAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	rf.lastUpdate = time.Now()

	if rf.role != Leader {
		return
	}

	if reply.Term > rf.currentTerm {
		// DPrintf("{Server %v, %v, Term %v, Last log index %v} Old leader discovers new term %v and convert to follower\n",
		// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), reply.Term)
		rf.currentTerm = reply.Term
		rf.convertRole(Follower)
		return
	}

	// DPrintf("{Server %v, %v, Term %v, Last log index %v} Target server %v nextIndex is %v, matchIndex is %v\n",
	// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), server, rf.nextIndex[server], rf.matchIndex[server])

	if !reply.Success {
		if rf.nextIndex[server] > 0 {
			rf.nextIndex[server]--
		}
		return
	}

	rf.nextIndex[server] = len(rf.logs)
	rf.matchIndex[server] = args.PrevLogIndex + len(args.Entries)
	// DPrintf("{Server %v, %v, Term %v, Last log index %v} Target server %v has match index %v log\n",
	// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), server, len(rf.logs)-1)

	serversCount := len(rf.peers)

	for n := rf.GetLastLogIndex(); n > rf.commitIndex; n-- {
		count := 1
		// DPrintf("{Server %v, %v, Term %v, Last log index %v} Check which log from %v to %v can be committed\n",
		// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), rf.GetLastLogIndex(), rf.commitIndex)
		if rf.logs[n].Term == rf.currentTerm {
			for i := 0; i < serversCount; i++ {
				if i != rf.me && rf.matchIndex[i] >= n {
					count++
				}
			}
		}
		// DPrintf("{Server %v, %v, Term %v, Last log index %v} How many server apply index %v log: %v\n",
		// 	rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex(), n, count)
		if count > len(rf.peers)/2 {
			rf.commitIndex = n
			go rf.applyLogs()
			break
		}
	}
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	term, isLeader := rf.GetState()
	if !isLeader {
		return -1, term, isLeader
	}

	entry := LogEntry{
		Term:    term,
		Command: command,
	}

	rf.mu.Lock()
	rf.logs = append(rf.logs, entry)
	index := rf.GetLastLogIndex()
	rf.mu.Unlock()

	return index, term, isLeader
}

// must be used in mutex
func (rf *Raft) convertRole(role Role) {
	if role == Follower {
		if rf.role == Leader {
			close(rf.heartChan)
		}
		rf.votedFor = -1
	} else if role == Leader {

		for i := range rf.nextIndex {
			rf.nextIndex[i] = len(rf.logs)
		}
		for i := range rf.matchIndex {
			rf.matchIndex[i] = -1
		}

		rf.heartChan = make(chan struct{})
		go rf.startHeartbeat()
	}

	rf.role = role
}

// Upon election: send initial empty AppendEntries RPCs(heartbeat)
// to each server; repeat during idle periods to prevent
// election timeouts (#5.2)
func (rf *Raft) startHeartbeat() {
	DPrintf("{Server %v, %v, Term %v, Last log index %v} Become Leader\n",
		rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex())
	// 每秒钟发送8次，测试程序要求leader每秒发送的心跳包次数不超过10次
	interval := time.Second / 8

	go func() {
		for !rf.killed() {
			select {
			case <-rf.heartChan:
				return
			default:
				_, isLeader := rf.GetState()

				// 当服务器是Leader时，发送心跳包
				if isLeader {

					// 并行向所有Follower send AppendEntries
					for i := range rf.peers {
						if i != rf.me {
							go func(server int) {
								args := rf.genAppendEntries(server)
								reply := AppendEntriesReply{}
								_, isLeader := rf.GetState()
								if isLeader && rf.sendAppendEntries(server, &args, &reply) {
									rf.handleAppendEntries(server, &args, &reply)
								}
							}(i)
						}
					}
				} else {
					return
				}
				// wait for next send AppendEntries
				time.Sleep(interval)
			}
		}
	}()
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// election timeout
func (rf *Raft) ticker() {
	for !rf.killed() {
		// Your code here (3A)

		// Check if a leader election should be started.
		ms := 600 + (rand.Int63() % 300)
		_, isLeader := rf.GetState()
		rf.mu.Lock()
		lastTime := rf.lastUpdate
		rf.mu.Unlock()

		// elecion timeout elapses: start new elections
		if !isLeader && time.Since(lastTime) > time.Duration(ms)*time.Millisecond {
			rf.startElection()
		}

		// pause for a random amount of time between 50 and 350
		// milliseconds.
		// ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

func (rf *Raft) checkState() {
	for Debug && !rf.killed() {
		DPrintf("{Server %v, %v, Term %v, Last log index %v}\n",
			rf.me, rf.role, rf.currentTerm, rf.GetLastLogIndex())
		time.Sleep(time.Second)
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.dead = 0
	rf.applyCh = applyCh

	// Your initialization code here (3A, 3B, 3C).
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.logs = []LogEntry{{Term: 0}}

	rf.commitIndex = 0
	rf.lastApplied = 0

	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))

	rf.role = Follower
	rf.lastUpdate = time.Now()

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()

	go rf.checkState()

	return rf
}
