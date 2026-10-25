package domain

type LocalBranch struct {
	Name, Hash, WorktreePath string
	Current                  bool
}

// BranchSwitchPlan pins both ends of the reviewed local switch.
type BranchSwitchPlan struct {
	WorkspacePath, From, FromHash, Target, TargetHash string
}
