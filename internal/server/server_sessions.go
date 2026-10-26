package server

import (
	"time"
)

// ActiveSessionCount returns the number of active backup sessions.
func (s *Server) ActiveSessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// completedSession holds a recently-finished session's info so SSE clients can display completion.
type completedSession struct {
	Info      SessionInfo
	ExpiresAt time.Time
}

// SessionInfo holds summary info about an active session for external consumers.
type SessionInfo struct {
	ID             string
	AgentName      string
	Hostname       string
	RepoName       string
	JobID          string
	BackupType     string
	StartTime      time.Time
	BytesAdded     uint64
	FilesProcessed uint64
	FilesNew       uint64
	Phase          string
	LastActivity   time.Time
}

func sessionToInfo(sess *Session) SessionInfo {
	bytes, files, filesNew, phase, lastAct := sess.Progress()
	return SessionInfo{
		ID:             sess.ID,
		AgentName:      sess.AgentName,
		Hostname:       sess.Hostname,
		RepoName:       sess.RepoName,
		JobID:          sess.JobID,
		BackupType:     sess.BackupType,
		StartTime:      sess.startTime,
		BytesAdded:     bytes,
		FilesProcessed: files,
		FilesNew:       filesNew,
		Phase:          phase,
		LastActivity:   lastAct,
	}
}

// SessionsByRepo returns active sessions for a given repo name.
func (s *Server) SessionsByRepo(repoName string) []SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []SessionInfo
	for _, sess := range s.sessions {
		if sess.RepoName == repoName {
			result = append(result, sessionToInfo(sess))
		}
	}
	return result
}

// AllSessions returns info for all active and recently completed sessions.
func (s *Server) AllSessions() []SessionInfo {
	s.mu.Lock()
	result := make([]SessionInfo, 0, len(s.sessions))
	for _, sess := range s.sessions {
		result = append(result, sessionToInfo(sess))
	}
	s.mu.Unlock()

	// Include recently completed sessions so SSE clients can display completion.
	s.completedMu.Lock()
	now := time.Now()
	for id, cs := range s.completedSessions {
		if now.After(cs.ExpiresAt) {
			delete(s.completedSessions, id)
			continue
		}
		result = append(result, cs.Info)
	}
	s.completedMu.Unlock()

	return result
}

// SessionByJobID returns the active session for the given job ID, or nil.
func (s *Server) SessionByJobID(jobID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.JobID == jobID {
			return sess
		}
	}
	return nil
}

// CancelSessionByJobID removes the active session for the given job ID.
// Returns true if a session was found and cancelled.
func (s *Server) CancelSessionByJobID(jobID string) bool {
	s.mu.Lock()
	var sessionID string
	for id, sess := range s.sessions {
		if sess.JobID == jobID {
			sessionID = id
			break
		}
	}
	if sessionID == "" {
		s.mu.Unlock()
		return false
	}
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	return true
}

// AbortSessionsByRepo cancels and removes all active sessions for a repo.
func (s *Server) AbortSessionsByRepo(repoName string) int {
	s.mu.Lock()
	var toRemove []string
	for id, sess := range s.sessions {
		if sess.RepoName == repoName {
			toRemove = append(toRemove, id)
		}
	}
	for _, id := range toRemove {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	return len(toRemove)
}

// Counters returns the current cumulative server counters.
func (s *Server) Counters() (completed, failed, bytesRecv, chunksRecv uint64) {
	return s.backupsCompleted.Load(),
		s.backupsFailed.Load(),
		s.bytesReceived.Load(),
		s.chunksReceived.Load()
}
