package proto

// Handout is the work given to an external AI by wait or next_item.
type Handout struct {
	Run     int64  `json:"run"`
	Kind    string `json:"kind"`
	Task    string `json:"task,omitempty"`
	Folder  string `json:"folder"`
	Context string `json:"context"`
	Text    string `json:"text"`
}
