package v1

type WatchInstruction struct {
	PortForwardAction *PortForwardAction `json:"portForwardAction,omitempty"`
	SyncVMsAction     *SyncVMsAction     `json:"syncVMsAction,omitempty"`
	ResolveIPAction   *ResolveIPAction   `json:"resolveIPAction,omitempty"`
	ExecAction        *ExecAction        `json:"execAction,omitempty"`
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

type ExecAction struct {
	Session     string        `json:"session"`
	VMUID       string        `json:"vmUID"`
	Command     string        `json:"command"`
	Args        []string      `json:"args"`
	Interactive bool          `json:"interactive"`
	TTY         bool          `json:"tty"`
	Terminal    *TerminalSize `json:"terminal,omitempty"`
}

type TerminalSize struct {
	Rows uint32 `json:"rows"`
	Cols uint32 `json:"cols"`
}
