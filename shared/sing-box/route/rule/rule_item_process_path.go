package rule

import (
	"strings"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/process"
)

var _ RuleItem = (*ProcessPathItem)(nil)

type ProcessPathItem struct {
	processes       []string
	processMap      map[string]bool
	includeChildren bool
}

func NewProcessPathItem(processNameList []string, includeChildren ...bool) *ProcessPathItem {
	rule := &ProcessPathItem{
		processes:       processNameList,
		includeChildren: len(includeChildren) > 0 && includeChildren[0],
		processMap:      make(map[string]bool),
	}
	for _, processName := range processNameList {
		rule.processMap[processName] = true
	}
	return rule
}

func (r *ProcessPathItem) Match(metadata *adapter.InboundContext) bool {
	if metadata.ProcessInfo == nil || metadata.ProcessInfo.ProcessPath == "" {
		return false
	}
	if r.processMap[metadata.ProcessInfo.ProcessPath] {
		return true
	}
	return r.includeChildren && process.MatchAncestorProcessPath(metadata.ProcessInfo.ProcessID, r.processMap)
}

func (r *ProcessPathItem) String() string {
	var description string
	pLen := len(r.processes)
	if pLen == 1 {
		description = "process_path=" + r.processes[0]
	} else {
		description = "process_path=[" + strings.Join(r.processes, " ") + "]"
	}
	if r.includeChildren {
		description += " (с дочерними процессами)"
	}
	return description
}
