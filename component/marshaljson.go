package component

import "encoding/json"

// Implement the MarshalJSON() method for stage type
func (s stage) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Implement the MarshalJSON() method for state type
func (s state) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Implement the MarshalJSON() method for controlMsg type
func (c controlMsg) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}
