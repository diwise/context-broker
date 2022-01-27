package types

//BaseEntity contains the required base properties an Entity must have
type BaseEntity struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Context []string `json:"@context"`
}
