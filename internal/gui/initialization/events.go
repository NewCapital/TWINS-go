package initialization

// InitProgress represents the progress of initialization
type InitProgress struct {
	Step        string `json:"step"`           // Current step identifier
	Description string `json:"description"`    // Human-readable description
	Progress    int    `json:"progress"`       // Progress percentage (0-100)
	TotalSteps  int    `json:"totalSteps"`     // Total number of steps
	CurrentStep int    `json:"currentStep"`    // Current step number
	IsComplete  bool   `json:"isComplete"`     // Whether initialization is complete
	Error       string `json:"error,omitempty"` // Error message if any
}
