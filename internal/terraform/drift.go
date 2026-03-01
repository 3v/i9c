package terraform

type Action string

const (
	ActionCreate            Action = "create"
	ActionUpdate            Action = "update"
	ActionDelete            Action = "delete"
	ActionReplace           Action = "replace"
	ActionCreateThenDelete  Action = "create-then-delete"
	ActionDeleteThenReplace Action = "delete-then-replace"
	ActionRead              Action = "read"
	ActionNoOp              Action = "no-op"
)

type ResourceChange struct {
	Address      string
	Type         string
	Name         string
	ProviderName string
	Action       Action
	Before       map[string]interface{}
	After        map[string]interface{}
}

type DriftResult struct {
	Changes    []ResourceChange
	HasChanges bool
	RawPlan    []byte
	Error      string
}

func (a Action) IsDrift() bool {
	switch a {
	case ActionCreate, ActionUpdate, ActionDelete, ActionReplace,
		ActionCreateThenDelete, ActionDeleteThenReplace:
		return true
	}
	return false
}
