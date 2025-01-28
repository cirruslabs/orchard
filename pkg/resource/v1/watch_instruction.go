package v1

type WatchInstruction struct {
	PortForwardAction *PortForwardAction `json:"portForwardAction,omitempty"`
	SyncVMsAction     *SyncVMsAction     `json:"syncVMsAction,omitempty"`
	ResolveIPAction   *ResolveIPAction   `json:"resolveIPAction,omitempty"`
}

type PortForwardAction struct {
	Session string `json:"session"`
	VMUID   string `json:"vmUID"`
	Port    uint16 `json:"port"`
}

type SyncVMsAction struct {
	// nothing for now
}

type ResolveIPAction struct {
	Session string `json:"session"`
	VMUID   string `json:"vmUID"`
}
