// Code generated by "stringer -type=stage"; DO NOT EDIT.

package component

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Submitted-0]
	_ = x[Preinitializing-1]
	_ = x[Preinitialized-2]
	_ = x[Initializing-3]
	_ = x[Initialized-4]
	_ = x[Starting-5]
	_ = x[Restarting-6]
	_ = x[Started-7]
	_ = x[Stopping-8]
	_ = x[Stopped-9]
	_ = x[Tearingdown-10]
	_ = x[Teareddown-11]
	_ = x[Aborting-12]
}

const _stage_name = "SubmittedPreinitializingPreinitializedInitializingInitializedStartingRestartingStartedStoppingStoppedTearingdownTeareddownAborting"

var _stage_index = [...]uint8{0, 9, 24, 38, 50, 61, 69, 79, 86, 94, 101, 112, 122, 130}

func (i stage) String() string {
	if i < 0 || i >= stage(len(_stage_index)-1) {
		return "stage(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _stage_name[_stage_index[i]:_stage_index[i+1]]
}
