// Code generated by "stringer -type=controlMsg component.go"; DO NOT EDIT.

package component

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[EnablePeerMessaging-0]
	_ = x[DisablePeerMessaging-1]
	_ = x[Restart-2]
	_ = x[RestartAfter-3]
	_ = x[RestartMmux-4]
	_ = x[Shutdown-5]
	_ = x[ShutdownAfter-6]
	_ = x[Cancel-7]
	_ = x[CancelAfter-8]
}

const _controlMsg_name = "EnablePeerMessagingDisablePeerMessagingRestartRestartAfterRestartMmuxShutdownShutdownAfterCancelCancelAfter"

var _controlMsg_index = [...]uint8{0, 19, 39, 46, 58, 69, 77, 90, 96, 107}

func (i controlMsg) String() string {
	if i < 0 || i >= controlMsg(len(_controlMsg_index)-1) {
		return "controlMsg(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _controlMsg_name[_controlMsg_index[i]:_controlMsg_index[i+1]]
}
