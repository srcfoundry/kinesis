// Code generated by "stringer -type=state"; DO NOT EDIT.

package component

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Inactive-0]
	_ = x[Active-1]
}

const _state_name = "InactiveActive"

var _state_index = [...]uint8{0, 8, 14}

func (i state) String() string {
	if i < 0 || i >= state(len(_state_index)-1) {
		return "state(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _state_name[_state_index[i]:_state_index[i+1]]
}
