package domain

type LocalBranch struct {
	Name, Hash, WorktreePath string
	Current                  bool
}

// BranchSwitchPlan pins both ends of the reviewed local switch.
type BranchSwitchPlan struct {
	WorkspacePath, From, FromHash, Target, TargetHash string
}

// BranchDeletePlan pins the reviewed local branch and, when it exists, the
// remote branch deleted with it (VORQ-020).
type BranchDeletePlan struct {
	WorkspacePath, Branch, Hash, Base string
	Merged                            bool // local tip reachable from the base
	Unpublished                       int  // commits on no remote
	Remote, RemoteHash                string
	RemoteMerged                      bool
}

// NeedsConfirm is true when deleting could lose commits: the typed
// GuardOverrideWord is then required.
func (p BranchDeletePlan) NeedsConfirm() bool {
	return !p.Merged || (p.RemoteHash != "" && !p.RemoteMerged)
}

type BranchDeleteResult struct {
	Plan          BranchDeletePlan
	RemoteDeleted bool
}
