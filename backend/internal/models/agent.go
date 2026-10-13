package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AgentSettings is a user's configuration for the Agents feature: the
// OpenRouter key and default model the agents call, and the global prompt
// every agent gets in addition to its own.
type AgentSettings struct {
	OpenRouterKey string `bson:"openRouterKey,omitempty" json:"-"`
	Model         string `bson:"model,omitempty" json:"model"`
	GlobalPrompt  string `bson:"globalPrompt,omitempty" json:"globalPrompt"`
}

// Agent schedule kinds.
const (
	AgentScheduleManual   = "manual"   // runs only when started by hand
	AgentScheduleInterval = "interval" // every IntervalMinutes
	AgentScheduleHourly   = "hourly"   // every hour at Minute
	AgentScheduleDaily    = "daily"    // at each of Times, on Weekdays (empty = every day)
)

// AgentSchedule says when an agent runs. Times and weekdays are read in
// Timezone (an IANA name, default Europe/Berlin).
type AgentSchedule struct {
	Kind            string   `bson:"kind" json:"kind"`
	IntervalMinutes int      `bson:"intervalMinutes,omitempty" json:"intervalMinutes,omitempty"`
	Minute          int      `bson:"minute,omitempty" json:"minute,omitempty"`
	Times           []string `bson:"times,omitempty" json:"times,omitempty"`       // "HH:MM"
	Weekdays        []int    `bson:"weekdays,omitempty" json:"weekdays,omitempty"` // 0 = Monday
	Timezone        string   `bson:"timezone,omitempty" json:"timezone,omitempty"`
}

// Agent is a prompt that runs on a schedule with the user's MCP tools.
type Agent struct {
	ID       primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID   primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID   *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Name     string              `bson:"name" json:"name"`
	Prompt   string              `bson:"prompt" json:"prompt"`
	Enabled  bool                `bson:"enabled" json:"enabled"`
	Schedule AgentSchedule       `bson:"schedule" json:"schedule"`
	// Model overrides the user's default model; empty uses the default.
	Model    string `bson:"model,omitempty" json:"model,omitempty"`
	MaxSteps int    `bson:"maxSteps,omitempty" json:"maxSteps,omitempty"`

	NextRunAt    *time.Time `bson:"nextRunAt,omitempty" json:"nextRunAt,omitempty"`
	Running      bool       `bson:"running" json:"running"`
	RunStartedAt *time.Time `bson:"runStartedAt,omitempty" json:"runStartedAt,omitempty"`
	LastRunAt    *time.Time `bson:"lastRunAt,omitempty" json:"lastRunAt,omitempty"`
	LastStatus   string     `bson:"lastStatus,omitempty" json:"lastStatus,omitempty"`
	LastError    string     `bson:"lastError,omitempty" json:"lastError,omitempty"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// Agent run statuses.
const (
	AgentRunRunning = "running"
	AgentRunOK      = "ok"
	AgentRunError   = "error"
	AgentRunLimit   = "limit" // stopped at the step limit
	AgentRunStopped = "stopped"
)

// AgentRunStep is one entry of a run's log: something the model said, or a
// tool it called and what came back (both truncated).
type AgentRunStep struct {
	At      time.Time `bson:"at" json:"at"`
	Kind    string    `bson:"kind" json:"kind"` // "message" | "tool"
	Text    string    `bson:"text,omitempty" json:"text,omitempty"`
	Tool    string    `bson:"tool,omitempty" json:"tool,omitempty"`
	Args    string    `bson:"args,omitempty" json:"args,omitempty"`
	Result  string    `bson:"result,omitempty" json:"result,omitempty"`
	IsError bool      `bson:"isError,omitempty" json:"isError,omitempty"`
}

// AgentRun records one run of an agent. Runs expire after ExpiresAt.
type AgentRun struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AgentID          primitive.ObjectID `bson:"agentId" json:"agentId"`
	UserID           primitive.ObjectID `bson:"userId" json:"userId"`
	Trigger          string             `bson:"trigger" json:"trigger"` // "schedule" | "manual"
	Model            string             `bson:"model" json:"model"`
	Status           string             `bson:"status" json:"status"`
	StartedAt        time.Time          `bson:"startedAt" json:"startedAt"`
	FinishedAt       *time.Time         `bson:"finishedAt,omitempty" json:"finishedAt,omitempty"`
	Steps            []AgentRunStep     `bson:"steps" json:"steps,omitempty"`
	ToolCalls        int                `bson:"toolCalls" json:"toolCalls"`
	Summary          string             `bson:"summary,omitempty" json:"summary,omitempty"`
	Error            string             `bson:"error,omitempty" json:"error,omitempty"`
	PromptTokens     int                `bson:"promptTokens" json:"promptTokens"`
	CompletionTokens int                `bson:"completionTokens" json:"completionTokens"`
	Cost             float64            `bson:"cost" json:"cost"`
	ExpiresAt        time.Time          `bson:"expiresAt" json:"-"`
}
