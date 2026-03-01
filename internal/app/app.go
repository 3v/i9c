package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"i9c/internal/agent"
	"i9c/internal/agent/providers"
	iaws "i9c/internal/aws"
	"i9c/internal/aws/resources"
	"i9c/internal/aws/resources/builtin"
	"i9c/internal/codexbridge"
	"i9c/internal/config"
	"i9c/internal/iac"
	ilog "i9c/internal/logs"
	"i9c/internal/state"
	"i9c/internal/terraform"
	"i9c/internal/tui"
	"i9c/internal/tui/views"
	"i9c/internal/watcher"
)

type App struct {
	cfg               *config.Config
	clientManager     *iaws.ClientManager
	registry          *resources.Registry
	tfRunner          *terraform.Runner
	watcher           *watcher.Watcher
	watcherMu         sync.Mutex
	awsAgent          *agent.Agent
	tfAgent           *agent.Agent
	agentMu           sync.RWMutex
	program           *tea.Program
	dirChangeCh       chan string
	advisorSendCh     chan string
	advisorCancelCh   chan struct{}
	llmConfigChangeCh chan config.LLMConfig
	iacConfigChangeCh chan config.TerraformConfig
	awsActionCh       chan tui.AWSAction

	resourceAdapter *ResourceAdapter
	toolExecutor    *agent.ToolExecutor
	logsHub         *ilog.Hub
	stateStore      state.ProfileSelectionStore

	cachedDrift     string
	cachedDriftMu   sync.RWMutex
	cachedResources []views.ResourceEntry
	cachedResMu     sync.RWMutex
	lastScan        time.Time
	prereqStatus    string
	prereqSummary   string
	statsMu         sync.RWMutex
	driftRunMu      sync.Mutex
	driftRunning    bool
	loginMu         sync.Mutex
	loginCancel     context.CancelFunc
	loginProfile    string
	loginSeq        uint64
	activeLoginID   uint64
	codexBridge     *codexbridge.AppServerBridge
	codexMu         sync.RWMutex
	codexPendingMu  sync.Mutex
	codexPending    *codexPendingRequest
	codexStreaming  bool
	advisorRunMu    sync.Mutex
	advisorCancelFn context.CancelFunc
	advisorRunSeq   uint64
	activeAdvisorID uint64
	mcpPrimary      string
	mcpFallback     string
	mcpActive       string
	mcpCodexState   string
}

type codexPendingRequest struct {
	Key  string
	Kind string
}

func Run(cfg *config.Config) error {
	if err := cfg.EnsureLocalDirs(); err != nil {
		return fmt.Errorf("ensuring local dirs: %w", err)
	}

	a := &App{
		cfg:               cfg,
		dirChangeCh:       make(chan string, 1),
		advisorSendCh:     make(chan string, 8),
		advisorCancelCh:   make(chan struct{}, 4),
		llmConfigChangeCh: make(chan config.LLMConfig, 1),
		iacConfigChangeCh: make(chan config.TerraformConfig, 1),
		awsActionCh:       make(chan tui.AWSAction, 8),
		logsHub:           ilog.NewHub(1000),
		stateStore:        state.NewYAMLProfileSelectionStore(cfg.Paths.Root),
	}
	a.logf(ilog.ChannelApp, "i9c starting")
	a.mcpPrimary = "managed aws mcp"
	a.mcpFallback = "local aws mcp"
	a.mcpActive = "in-process aws sdk discovery"
	a.mcpCodexState = "disconnected"
	configChanged := false
	codexAvailable := commandAvailable("codex")
	if cfg.LLM.Provider == "" {
		if codexAvailable {
			cfg.LLM.Provider = "codex"
			cfg.LLM.Model = normalizeCodexModel(cfg.LLM.Model)
			configChanged = true
			a.logf(ilog.ChannelAgent, "defaulting LLM provider to codex (codex command detected)")
		} else {
			cfg.LLM.Provider = "openai"
			if strings.TrimSpace(cfg.LLM.Model) == "" {
				cfg.LLM.Model = "gpt-4o"
			}
			configChanged = true
			a.logf(ilog.ChannelAgent, "defaulting LLM provider to openai (codex command not found)")
		}
	}
	if cfg.LLM.Provider == "codex" && !codexAvailable {
		cfg.LLM.Provider = "openai"
		if strings.TrimSpace(cfg.LLM.Model) == "" || strings.Contains(strings.ToLower(cfg.LLM.Model), "codex") {
			cfg.LLM.Model = "gpt-4o"
		}
		configChanged = true
		a.logf(ilog.ChannelAgent, "codex command not found; falling back to openai provider")
	}
	if cfg.LLM.Provider == "openai" && cfg.LLM.ResolveAPIKey() == "" && codexAvailable {
		cfg.LLM.Provider = "codex"
		cfg.LLM.Model = normalizeCodexModel(cfg.LLM.Model)
		configChanged = true
		a.logf(ilog.ChannelAgent, "No OpenAI key found; defaulting LLM provider to codex")
	}
	if cfg.LLM.Provider == "codex" {
		normalized := normalizeCodexModel(cfg.LLM.Model)
		if normalized != cfg.LLM.Model {
			cfg.LLM.Model = normalized
			configChanged = true
			a.logf(ilog.ChannelAgent, "normalized codex model to %s", normalized)
		}
	}
	if configChanged {
		if err := cfg.Save(""); err != nil {
			a.logf(ilog.ChannelApp, "failed to persist normalized config: %v", err)
		}
	}

	a.clientManager = iaws.NewClientManager(cfg)
	a.registry = resources.NewRegistry(cfg)
	a.registry.RegisterBuiltin(builtin.NewEC2Provider())
	a.registry.RegisterBuiltin(builtin.NewVPCProvider())
	a.registry.RegisterBuiltin(builtin.NewS3Provider())
	a.registry.RegisterBuiltin(builtin.NewIAMProvider())
	a.registry.RegisterBuiltin(builtin.NewRDSProvider())
	a.registry.RegisterBuiltin(builtin.NewEKSProvider())

	a.tfRunner = terraform.NewRunner(cfg.Terraform.Binary, cfg.IACDir)

	a.resourceAdapter = NewResourceAdapter(a.registry, a.clientManager)
	a.toolExecutor = &agent.ToolExecutor{
		ResourceLister: a.resourceAdapter,
		IACDir:         cfg.IACDir,
		IACBinary:      cfg.Terraform.Binary,
		DriftData:      a.getCachedDrift,
	}

	model := tui.NewModel(cfg, a.logsHub)
	model.SetDirChangeNotify(a.dirChangeCh)
	model.SetAdvisorSendCh(a.advisorSendCh)
	model.SetAdvisorCancelCh(a.advisorCancelCh)
	model.SetLLMConfigChangeCh(a.llmConfigChangeCh)
	model.SetIaCConfigChangeCh(a.iacConfigChangeCh)
	model.SetAWSActionCh(a.awsActionCh)
	programOpts := []tea.ProgramOption{tea.WithMouseCellMotion()}
	if os.Getenv("I9C_NO_ALT_SCREEN") != "1" {
		programOpts = append(programOpts, tea.WithAltScreen())
	} else {
		a.logf(ilog.ChannelApp, "alt-screen disabled via I9C_NO_ALT_SCREEN=1")
	}
	a.program = tea.NewProgram(model, programOpts...)
	if err := a.applyLLMMode(cfg.LLM); err != nil {
		a.logf(ilog.ChannelAgent, "LLM provider not configured: %v", err)
	}
	a.sendMCPUpdate("app initialized", "")

	go a.backgroundInit()
	go a.startWatcher()
	go a.listenDirChanges()
	go a.listenAdvisorMessages()
	go a.listenAdvisorCancels()
	go a.listenLLMConfigChanges()
	go a.listenIaCConfigChanges()
	go a.listenAWSActions()
	go a.startDriftScheduler()

	if _, err := a.program.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}
	return nil
}

func (a *App) logf(channel, f string, args ...any) {
	a.logsHub.Add(channel, fmt.Sprintf(f, args...))
}

func (a *App) sendMCPUpdate(event, errText string) {
	if a.program == nil {
		return
	}
	msg := views.MCPUpdateMsg{
		ConfiguredPrimary:  a.mcpPrimary,
		ConfiguredFallback: a.mcpFallback,
		ActiveBackend:      a.mcpActive,
		CodexConnection:    a.mcpCodexState,
		Event:              event,
		Error:              errText,
		At:                 time.Now(),
	}
	// Avoid startup deadlock when updates are emitted before Program.Run starts.
	go a.program.Send(msg)
}

func (a *App) autoDetectBinary() string {
	if a.cfg.IACDir == "" {
		return a.toolExecutor.IACBinary
	}
	desired := strings.TrimSpace(a.cfg.Terraform.Binary)
	detected := strings.TrimSpace(terraform.DetectBinary(a.cfg.IACDir))
	if detected != "" {
		desired = detected
	}
	if desired == "" {
		desired = "tofu"
	}
	if detected != "" && desired != a.toolExecutor.IACBinary {
		a.logf(ilog.ChannelApp, "auto-detected IaC tool from folder: %s (config: %s)", desired, a.cfg.Terraform.Binary)
	}
	if desired != a.toolExecutor.IACBinary || strings.TrimSpace(a.tfRunner.Binary()) == "" {
		binPath, err := iac.EnsureBinary(desired, a.cfg.Terraform.Version)
		if err != nil {
			a.logf(ilog.ChannelApp, "could not resolve detected binary %s: %v", desired, err)
			return a.toolExecutor.IACBinary
		}
		a.tfRunner.SetBinary(binPath)
		a.toolExecutor.IACBinary = desired
	}
	return a.toolExecutor.IACBinary
}

func (a *App) backgroundInit() {
	ctx := context.Background()
	st, _ := a.stateStore.Load()
	if a.cfg.IACDir == "" && st.LastIACDir != "" {
		a.cfg.IACDir = st.LastIACDir
		a.tfRunner.SetWorkDir(st.LastIACDir)
		a.toolExecutor.IACDir = st.LastIACDir
	}

	binPath, err := iac.EnsureBinary(a.cfg.Terraform.Binary, a.cfg.Terraform.Version)
	if err != nil {
		a.logf(ilog.ChannelApp, "IaC binary check: %v", err)
	} else {
		a.tfRunner.SetBinary(binPath)
	}

	a.autoDetectBinary()
	a.logStartupPrerequisites()

	if err := a.clientManager.Initialize(ctx); err != nil {
		a.logf(ilog.ChannelSystem, "AWS client init: %v", err)
	}
	if st.LastProfile != "" {
		_ = a.clientManager.SelectProfile(st.LastProfile)
	}

	profiles := a.clientManager.Profiles()
	active := a.clientManager.ActiveProfile()
	a.program.Send(tui.ProfileCatalogMsg{Profiles: profiles, ActiveProfile: active})
	a.sendDashboardUpdate(time.Now())
	a.autoPromptSSOLoginIfNeeded(profiles, active)

	go a.refreshResources(ctx)
	go a.refreshDrift(ctx)
}

func (a *App) sendDashboardUpdate(scanTime time.Time) {
	a.cachedResMu.RLock()
	resourceCount := len(a.cachedResources)
	a.cachedResMu.RUnlock()

	a.cachedDriftMu.RLock()
	driftCount := 0
	if a.cachedDrift != "" && a.cachedDrift != "[]" {
		var entries []views.DriftEntry
		if err := json.Unmarshal([]byte(a.cachedDrift), &entries); err == nil {
			driftCount = len(entries)
		}
	}
	a.cachedDriftMu.RUnlock()

	if scanTime.IsZero() {
		a.statsMu.RLock()
		scanTime = a.lastScan
		a.statsMu.RUnlock()
	}
	if scanTime.IsZero() {
		scanTime = time.Now()
	}
	a.statsMu.Lock()
	a.lastScan = scanTime
	prereqStatus := a.prereqStatus
	prereqSummary := a.prereqSummary
	a.statsMu.Unlock()

	a.program.Send(views.DashboardUpdateMsg{
		DriftCount:    driftCount,
		ResourceCount: resourceCount,
		ProfileCount:  len(a.clientManager.ProfileNames()),
		ActiveProfile: a.clientManager.ActiveProfile(),
		BackendMode:   "managed/local",
		CacheHealth:   "ok",
		Runtime:       runtime.GOOS + "/" + runtime.GOARCH,
		PrereqStatus:  prereqStatus,
		PrereqSummary: prereqSummary,
		LastScan:      scanTime,
	})
}

func (a *App) autoPromptSSOLoginIfNeeded(profiles []iaws.ProfileInfo, active string) {
	if active != "" || !a.cfg.AWS.AutoSSOLoginPrompt {
		return
	}
	byName := make(map[string]iaws.ProfileInfo, len(profiles))
	for _, p := range profiles {
		byName[p.Name] = p
	}

	candidates := []string{}
	if env := os.Getenv("AWS_PROFILE"); env != "" {
		candidates = append(candidates, env)
	}
	if a.cfg.AWS.DefaultProfile != "" {
		candidates = append(candidates, a.cfg.AWS.DefaultProfile)
	}
	candidates = append(candidates, "default")

	seen := map[string]bool{}
	for _, name := range candidates {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		p, ok := byName[name]
		if !ok {
			continue
		}
		switch p.Status {
		case iaws.StatusNoSession, iaws.StatusExpired, iaws.StatusUnknown:
			a.program.Send(views.ProfileActionStatusMsg{Text: fmt.Sprintf("Auto-login prompt: attempting %s", name)})
			a.startProfileLogin(name)
			return
		}
	}
	a.program.Send(views.ProfileActionStatusMsg{Text: "No live profile. Open Profiles [p] and run login [l].", IsError: true})
}

func (a *App) refreshResources(ctx context.Context) {
	allResources, err := a.registry.ListAll(ctx, a.clientManager)
	if err != nil {
		a.logf(ilog.ChannelSystem, "resource listing: %v", err)
		return
	}
	a.resourceAdapter.SetCachedResources(allResources)

	var entries []views.ResourceEntry
	for _, r := range allResources {
		entries = append(entries, views.ResourceEntry{
			Profile:    r.Profile,
			Service:    r.Service,
			Type:       r.Type,
			ID:         r.ID,
			Name:       r.Name,
			Region:     r.Region,
			Properties: r.Properties,
		})
	}

	a.cachedResMu.Lock()
	a.cachedResources = entries
	a.cachedResMu.Unlock()

	a.program.Send(views.ResourcesUpdateMsg{Entries: entries})
	a.sendDashboardUpdate(time.Now())
}

func (a *App) refreshDrift(ctx context.Context) {
	if !a.tryStartDriftRun() {
		return
	}
	defer a.finishDriftRun()

	if a.cfg.IACDir == "" {
		return
	}
	activeBinary := a.autoDetectBinary()
	if activeBinary == "" {
		a.logf(ilog.ChannelDrift, "no IaC binary resolved; skipping drift run")
		return
	}
	if !a.cfg.Terraform.AutoInit && !isIACInitialized(a.cfg.IACDir) {
		a.logf(ilog.ChannelDrift, "IaC folder is not initialized; enable terraform.auto_init or run '%s init' manually", activeBinary)
		return
	}

	if a.cfg.Terraform.AutoInit {
		if err := a.tfRunner.Init(ctx); err != nil {
			a.logf(ilog.ChannelDrift, "%s init: %v", activeBinary, err)
		}
	}

	result, err := a.tfRunner.Plan(ctx)
	if err != nil {
		a.logf(ilog.ChannelDrift, "%s plan: %v", activeBinary, err)
		return
	}
	if result.Error != "" {
		a.logf(ilog.ChannelDrift, "%s plan error: %s", activeBinary, result.Error)
		return
	}

	var entries []views.DriftEntry
	for _, change := range result.Changes {
		entries = append(entries, views.DriftEntry{
			Address: change.Address,
			Type:    change.Type,
			Action:  views.DriftAction(change.Action),
			Before:  change.Before,
			After:   change.After,
		})
	}
	driftJSON, _ := json.Marshal(entries)
	a.cachedDriftMu.Lock()
	a.cachedDrift = string(driftJSON)
	a.cachedDriftMu.Unlock()

	a.program.Send(views.DriftUpdateMsg{Entries: entries})
	a.sendDashboardUpdate(time.Now())
}

func (a *App) tryStartDriftRun() bool {
	a.driftRunMu.Lock()
	defer a.driftRunMu.Unlock()
	if a.driftRunning {
		return false
	}
	a.driftRunning = true
	return true
}

func (a *App) finishDriftRun() {
	a.driftRunMu.Lock()
	a.driftRunning = false
	a.driftRunMu.Unlock()
}

func (a *App) startDriftScheduler() {
	interval := time.Duration(a.cfg.Terraform.DriftCheckIntervalMin) * time.Minute
	t := time.NewTicker(interval)
	defer t.Stop()
	a.runDriftSchedulerTicks(t.C)
}

func (a *App) runDriftSchedulerTicks(ticks <-chan time.Time) {
	for range ticks {
		if a.cfg.IACDir == "" {
			continue
		}
		a.logf(ilog.ChannelDrift, "scheduled drift check")
		go a.refreshDrift(context.Background())
	}
}

func (a *App) getCachedDrift() string {
	a.cachedDriftMu.RLock()
	defer a.cachedDriftMu.RUnlock()
	return a.cachedDrift
}

func (a *App) startWatcher() {
	if a.cfg.IACDir == "" {
		return
	}
	a.watcherMu.Lock()
	w, err := watcher.New(a.cfg.IACDir)
	if err != nil {
		a.logf(ilog.ChannelApp, "watcher init: %v", err)
		a.watcherMu.Unlock()
		return
	}
	a.watcher = w
	a.watcherMu.Unlock()
	if err := w.Start(); err != nil {
		a.logf(ilog.ChannelApp, "watcher start: %v", err)
		return
	}
	for range w.Events() {
		ctx := context.Background()
		go a.refreshDrift(ctx)
	}
}

func (a *App) listenLLMConfigChanges() {
	for newCfg := range a.llmConfigChangeCh {
		if err := a.applyLLMMode(newCfg); err != nil {
			a.logf(ilog.ChannelAgent, "failed to create LLM provider: %v", err)
			a.program.Send(views.AdvisorResponseMsg{Content: fmt.Sprintf("\n\nFailed to switch LLM provider: %v", err), Done: true})
			continue
		}
		a.cfg.LLM = newCfg
		if err := a.cfg.Save(""); err != nil {
			a.logf(ilog.ChannelApp, "failed to save llm config: %v", err)
		}
		a.program.Send(views.AdvisorResponseMsg{Content: fmt.Sprintf("\n\nSwitched to %s / %s", newCfg.Provider, newCfg.Model), Done: true})
		a.logf(ilog.ChannelAgent, "LLM provider switched to %s / %s", newCfg.Provider, newCfg.Model)
	}
}

func (a *App) listenAdvisorMessages() {
	for userText := range a.advisorSendCh {
		if br := a.getCodexBridge(); br != nil {
			if a.handleCodexPendingReply(br, userText) {
				continue
			}
			a.program.Send(views.AdvisorBusyMsg{Busy: true})
			a.codexPendingMu.Lock()
			a.codexStreaming = true
			a.codexPendingMu.Unlock()
			a.program.Send(views.AdvisorResponseMsg{
				Content: "\n[i9c] Submitted to Codex. Waiting for response...",
				Done:    true,
			})
			a.program.Send(views.AdvisorActivityMsg{Text: "turn submitted", Running: true})
			a.logf(ilog.ChannelAgent, "codex turn submitted; waiting for response")
			if err := br.SendUserTurn(context.Background(), userText); err != nil {
				a.program.Send(views.AdvisorResponseMsg{Content: "Codex turn error: " + err.Error(), Done: true})
				a.program.Send(views.AdvisorBusyMsg{Busy: false})
				a.program.Send(views.AdvisorActivityMsg{Text: "turn request failed", Running: false})
				a.logf(ilog.ChannelAgent, "codex turn error: %v", err)
				a.codexPendingMu.Lock()
				a.codexStreaming = false
				a.codexPendingMu.Unlock()
			}
			continue
		}

		a.agentMu.RLock()
		currentAgent := a.awsAgent
		a.agentMu.RUnlock()
		if currentAgent == nil {
			a.program.Send(views.AdvisorResponseMsg{
				Content: "No LLM provider configured. Press [l] to configure LLM settings.",
				Done:    true,
			})
			continue
		}

		agentCtx := a.buildAgentContext()
		runCtx, cancel := context.WithCancel(context.Background())
		a.advisorRunMu.Lock()
		a.advisorRunSeq++
		runID := a.advisorRunSeq
		a.activeAdvisorID = runID
		a.advisorCancelFn = cancel
		a.advisorRunMu.Unlock()
		a.program.Send(views.AdvisorBusyMsg{Busy: true})
		a.program.Send(views.AdvisorResponseMsg{Content: "\n\n"})
		statusFn := func(status string) {
			a.program.Send(views.AdvisorResponseMsg{Content: fmt.Sprintf("\n🔧 %s\n", status)})
			a.logf(ilog.ChannelAgent, "%s", status)
		}
		result, err := currentAgent.ChatWithTools(runCtx, userText, agentCtx, a.toolExecutor, statusFn)
		a.advisorRunMu.Lock()
		if a.activeAdvisorID == runID {
			a.advisorCancelFn = nil
			a.activeAdvisorID = 0
		}
		a.advisorRunMu.Unlock()
		a.program.Send(views.AdvisorBusyMsg{Busy: false})
		if err != nil {
			a.program.Send(views.AdvisorResponseMsg{Content: "Error: " + err.Error(), Done: true})
			a.logf(ilog.ChannelAgent, "advisor error: %v", err)
			continue
		}
		a.program.Send(views.AdvisorResponseMsg{Content: result, Done: true})
	}
}

func (a *App) listenAdvisorCancels() {
	for range a.advisorCancelCh {
		cancelled := false
		if br := a.getCodexBridge(); br != nil {
			a.program.Send(views.AdvisorResponseMsg{Content: "\n[i9c] Cancel requested. Waiting for Codex...", Done: true})
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := br.InterruptActiveTurn(ctx); err == nil {
					a.logf(ilog.ChannelAgent, "codex turn cancel requested")
					a.logf(ilog.ChannelSystem, "codex turn cancel requested")
					a.program.Send(views.AdvisorResponseMsg{Content: "\n[i9c] Cancel signal sent to Codex.", Done: true})
				} else {
					a.logf(ilog.ChannelAgent, "codex cancel request not applied: %v", err)
					a.logf(ilog.ChannelSystem, "codex cancel request not applied: %v", err)
					a.program.Send(views.AdvisorResponseMsg{Content: "\n[i9c] Cancel not applied: " + err.Error(), Done: true})
				}
			}()
			continue
		}
		a.advisorRunMu.Lock()
		if a.advisorCancelFn != nil {
			a.advisorCancelFn()
			a.advisorCancelFn = nil
			a.activeAdvisorID = 0
			cancelled = true
		}
		a.advisorRunMu.Unlock()
		if cancelled {
			a.program.Send(views.AdvisorResponseMsg{Content: "\n[i9c] Cancel requested.", Done: true})
		} else {
			a.program.Send(views.AdvisorResponseMsg{Content: "\n[i9c] Nothing to cancel.", Done: true})
		}
		a.program.Send(views.AdvisorBusyMsg{Busy: false})
	}
}

func (a *App) startCodexBridge() {
	br := a.getCodexBridge()
	if br == nil {
		return
	}
	ctx := context.Background()
	if err := br.Start(ctx); err != nil {
		a.logf(ilog.ChannelAgent, "codex bridge start failed: %v", err)
		a.mcpCodexState = "error"
		a.sendMCPUpdate("", err.Error())
		a.program.Send(views.AdvisorResponseMsg{Content: "Codex bridge start failed: " + err.Error(), Done: true})
		return
	}
	a.mcpCodexState = "connected"
	a.mcpActive = "codex app-server"
	a.sendMCPUpdate("codex app-server connected", "")
	a.logf(ilog.ChannelAgent, "codex bridge started")
}

func (a *App) listenCodexBridgeEvents(br *codexbridge.AppServerBridge) {
	for ev := range br.Events() {
		switch ev.Type {
		case codexbridge.EventAgentDelta:
			a.program.Send(views.AdvisorResponseMsg{Content: ev.Text})
		case codexbridge.EventTurnDone:
			a.program.Send(views.AdvisorResponseMsg{Content: "\n[i9c] Turn completed.", Done: true})
			a.program.Send(views.AdvisorBusyMsg{Busy: false})
			a.program.Send(views.AdvisorActivityMsg{Text: "turn completed", Running: false})
			a.codexPendingMu.Lock()
			a.codexStreaming = false
			a.codexPendingMu.Unlock()
		case codexbridge.EventTurnFailed:
			if strings.Contains(strings.ToLower(ev.Text), "reconnect") {
				a.logf(ilog.ChannelAgent, "transient codex reconnect: %s", ev.Text)
				a.program.Send(views.AdvisorResponseMsg{Content: "\n[i9c] " + ev.Text, Done: true})
				a.program.Send(views.AdvisorActivityMsg{Text: ev.Text, Running: true})
				break
			}
			a.program.Send(views.AdvisorResponseMsg{Content: "\n\nCodex error: " + ev.Text, Done: true})
			a.program.Send(views.AdvisorBusyMsg{Busy: false})
			a.program.Send(views.AdvisorActivityMsg{Text: "turn failed", Running: false})
			a.codexPendingMu.Lock()
			a.codexStreaming = false
			a.codexPendingMu.Unlock()
		case codexbridge.EventApprovalReq:
			a.codexPendingMu.Lock()
			a.codexPending = &codexPendingRequest{Key: ev.RequestKey, Kind: "approval"}
			a.codexPendingMu.Unlock()
			a.program.Send(views.AdvisorBusyMsg{Busy: false})
			a.program.Send(views.AdvisorActivityMsg{Text: "awaiting approval", Running: false})
			a.program.Send(views.AdvisorResponseMsg{Content: "\n\n[Codex Approval] " + ev.Text + "\n", Done: true})
		case codexbridge.EventQuestionReq:
			a.codexPendingMu.Lock()
			a.codexPending = &codexPendingRequest{Key: ev.RequestKey, Kind: "question"}
			a.codexPendingMu.Unlock()
			a.program.Send(views.AdvisorBusyMsg{Busy: false})
			a.program.Send(views.AdvisorActivityMsg{Text: "awaiting user input", Running: false})
			a.program.Send(views.AdvisorResponseMsg{Content: "\n\n[Codex Question] " + ev.Text + "\nReply with your answer.\n", Done: true})
		case codexbridge.EventRequestClear:
			a.codexPendingMu.Lock()
			a.codexPending = nil
			a.codexPendingMu.Unlock()
		case codexbridge.EventInfo:
			a.logf(ilog.ChannelAgent, "%s", ev.Text)
			a.logf(ilog.ChannelSystem, "[codex] %s", ev.Text)
			a.sendMCPUpdate(ev.Text, "")
			if strings.HasPrefix(ev.Text, "progress: ") {
				step := strings.TrimPrefix(ev.Text, "progress: ")
				a.program.Send(views.AdvisorActivityMsg{Text: step, Running: true})
				a.program.Send(views.AdvisorResponseMsg{
					Content: "\n[i9c] " + step,
					Done:    true,
				})
			}
		case codexbridge.EventError:
			a.logf(ilog.ChannelAgent, "%s", ev.Text)
			a.logf(ilog.ChannelSystem, "[codex] %s", ev.Text)
			a.sendMCPUpdate("", ev.Text)
		}
	}
}

func (a *App) handleCodexPendingReply(br *codexbridge.AppServerBridge, userText string) bool {
	a.codexPendingMu.Lock()
	pending := a.codexPending
	a.codexPendingMu.Unlock()
	if pending == nil {
		return false
	}

	switch pending.Kind {
	case "approval":
		decision := mapApprovalDecision(userText)
		if decision == "" {
			a.program.Send(views.AdvisorResponseMsg{Content: "Invalid approval reply. Use: approve | session | decline | cancel", Done: true})
			return true
		}
		if err := br.RespondApproval(context.Background(), pending.Key, decision); err != nil {
			a.program.Send(views.AdvisorResponseMsg{Content: "Failed to send approval response: " + err.Error(), Done: true})
		} else {
			a.program.Send(views.AdvisorBusyMsg{Busy: true})
			a.program.Send(views.AdvisorActivityMsg{Text: "approval response sent", Running: true})
			a.program.Send(views.AdvisorResponseMsg{Content: "Approval response sent: " + decision, Done: true})
			a.codexPendingMu.Lock()
			a.codexPending = nil
			a.codexPendingMu.Unlock()
		}
		return true
	case "question":
		if err := br.RespondQuestion(context.Background(), pending.Key, userText); err != nil {
			a.program.Send(views.AdvisorResponseMsg{Content: "Failed to send answer: " + err.Error(), Done: true})
		} else {
			a.program.Send(views.AdvisorBusyMsg{Busy: true})
			a.program.Send(views.AdvisorActivityMsg{Text: "answer submitted", Running: true})
			a.program.Send(views.AdvisorResponseMsg{Content: "Answer submitted to Codex.", Done: true})
			a.codexPendingMu.Lock()
			a.codexPending = nil
			a.codexPendingMu.Unlock()
		}
		return true
	default:
		return false
	}
}

func (a *App) getCodexBridge() *codexbridge.AppServerBridge {
	a.codexMu.RLock()
	defer a.codexMu.RUnlock()
	return a.codexBridge
}

func (a *App) setCodexBridge(br *codexbridge.AppServerBridge) {
	a.codexMu.Lock()
	a.codexBridge = br
	a.codexMu.Unlock()
}

func (a *App) stopCodexBridge() {
	br := a.getCodexBridge()
	if br == nil {
		return
	}
	_ = br.Stop(context.Background())
	a.mcpCodexState = "disconnected"
	a.mcpActive = "in-process aws sdk discovery"
	a.sendMCPUpdate("codex app-server disconnected", "")
	a.setCodexBridge(nil)
	a.codexPendingMu.Lock()
	a.codexPending = nil
	a.codexPendingMu.Unlock()
}

func (a *App) applyLLMMode(cfg config.LLMConfig) error {
	if cfg.Provider == "codex" {
		cfg.Model = normalizeCodexModel(cfg.Model)
		if a.cfg != nil {
			a.cfg.LLM.Model = cfg.Model
		}
		a.agentMu.Lock()
		a.awsAgent = nil
		a.tfAgent = nil
		a.agentMu.Unlock()
		a.stopCodexBridge()
		cwd := a.cfg.IACDir
		if strings.TrimSpace(cwd) == "" {
			cwd = "."
		}
		br := codexbridge.NewAppServerBridge("codex", cwd, cfg.Model)
		a.setCodexBridge(br)
		go a.startCodexBridge()
		go a.listenCodexBridgeEvents(br)
		return nil
	}

	a.stopCodexBridge()
	llmProvider, err := providers.NewProvider(&cfg)
	if err != nil {
		return err
	}
	a.agentMu.Lock()
	a.awsAgent = agent.NewAgentWithModel(llmProvider, agent.AgentAWSAPI, cfg.Model)
	a.tfAgent = agent.NewAgentWithModel(llmProvider, agent.AgentTerraform, cfg.Model)
	a.agentMu.Unlock()
	return nil
}

func normalizeCodexModel(model string) string {
	m := strings.TrimSpace(strings.ToLower(model))
	if m == "" {
		return "gpt-5"
	}
	// ChatGPT-account Codex does not support gpt-4o in app-server mode.
	if strings.Contains(m, "gpt-4o") {
		return "gpt-5"
	}
	return model
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isIACInitialized(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	candidates := []string{
		filepath.Join(dir, ".terraform"),
		filepath.Join(dir, ".tofu"),
		filepath.Join(dir, ".terraform.lock.hcl"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil {
			if st.IsDir() || strings.HasSuffix(p, ".hcl") {
				return true
			}
		}
	}
	return false
}

func detectIACExtensions(dir string) (tfCount, tofuCount int, err error) {
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil {
		return 0, 0, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".tf":
			tfCount++
		case ".tofu":
			tofuCount++
		}
	}
	return tfCount, tofuCount, nil
}

func (a *App) logStartupPrerequisites() {
	awsOK := commandAvailable("aws")
	tenvOK := commandAvailable("tenv")
	codexOK := commandAvailable("codex")
	openAIKeyOK := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
	missing := []string{}
	warnings := []string{}
	if !awsOK {
		missing = append(missing, "aws")
	}
	if !tenvOK {
		warnings = append(warnings, "tenv")
	}
	if !codexOK && !openAIKeyOK {
		missing = append(missing, "codex-or-openai-key")
	}
	a.logf(ilog.ChannelSystem, "prereq check: runtime=%s/%s", runtime.GOOS, runtime.GOARCH)
	a.logf(ilog.ChannelSystem, "prereq check: aws cli=%t tenv=%t codex=%t OPENAI_API_KEY set=%t", awsOK, tenvOK, codexOK, openAIKeyOK)

	status := "OK"
	summaryParts := []string{}
	if len(missing) > 0 {
		status = fmt.Sprintf("MISSING(%d)", len(missing))
		summaryParts = append(summaryParts, "missing="+strings.Join(missing, ","))
	}
	if len(warnings) > 0 {
		if status == "OK" {
			status = fmt.Sprintf("WARN(%d)", len(warnings))
		}
		summaryParts = append(summaryParts, "warn="+strings.Join(warnings, ","))
	}

	if strings.TrimSpace(a.cfg.IACDir) == "" {
		summaryParts = append(summaryParts, "iac_dir=not-selected")
		a.logf(ilog.ChannelSystem, "prereq check: iac_dir not selected yet")
		a.statsMu.Lock()
		a.prereqStatus = status
		a.prereqSummary = strings.Join(summaryParts, " | ")
		a.statsMu.Unlock()
		a.sendDashboardUpdate(time.Now())
		return
	}
	if st, err := os.Stat(a.cfg.IACDir); err != nil || !st.IsDir() {
		if status == "OK" {
			status = "WARN(1)"
		}
		summaryParts = append(summaryParts, "iac_dir=unavailable")
		a.logf(ilog.ChannelSystem, "prereq check: iac_dir unavailable (%s)", a.cfg.IACDir)
		a.statsMu.Lock()
		a.prereqStatus = status
		a.prereqSummary = strings.Join(summaryParts, " | ")
		a.statsMu.Unlock()
		a.sendDashboardUpdate(time.Now())
		return
	}
	tfCount, tofuCount, err := detectIACExtensions(a.cfg.IACDir)
	if err != nil {
		if status == "OK" {
			status = "WARN(1)"
		}
		summaryParts = append(summaryParts, "iac_scan_error")
		a.logf(ilog.ChannelSystem, "prereq check: failed to inspect iac_dir: %v", err)
	} else {
		summaryParts = append(summaryParts, fmt.Sprintf(".tf=%d .tofu=%d", tfCount, tofuCount))
		a.logf(ilog.ChannelSystem, "prereq check: iac files .tf=%d .tofu=%d", tfCount, tofuCount)
	}
	if isIACInitialized(a.cfg.IACDir) {
		summaryParts = append(summaryParts, "init=yes")
		a.logf(ilog.ChannelSystem, "prereq check: iac init state=initialized")
	} else {
		summaryParts = append(summaryParts, "init=no")
		a.logf(ilog.ChannelSystem, "prereq check: iac init state=not initialized")
	}
	active := a.toolExecutor.IACBinary
	if strings.TrimSpace(active) == "" {
		active = a.cfg.Terraform.Binary
	}
	if active == "" {
		active = "tofu"
	}
	summaryParts = append(summaryParts, "drift="+active)
	a.logf(ilog.ChannelSystem, "prereq check: drift binary=%s (from file extensions when present)", active)
	a.statsMu.Lock()
	a.prereqStatus = status
	a.prereqSummary = strings.Join(summaryParts, " | ")
	a.statsMu.Unlock()
	a.sendDashboardUpdate(time.Now())
}

func mapApprovalDecision(in string) string {
	s := strings.ToLower(strings.TrimSpace(in))
	switch s {
	case "approve", "accept", "yes", "y":
		return "accept"
	case "session", "accept for session":
		return "acceptForSession"
	case "decline", "deny", "no", "n":
		return "decline"
	case "cancel":
		return "cancel"
	default:
		return ""
	}
}

func (a *App) buildAgentContext() *agent.Context {
	agentCtx := &agent.Context{}
	for _, p := range a.clientManager.Profiles() {
		agentCtx.Profiles = append(agentCtx.Profiles, agent.ProfileContext{Name: p.Name, Region: p.Region})
	}
	a.cachedResMu.RLock()
	agentCtx.ResourceCount = len(a.cachedResources)
	svcCounts := make(map[string]int)
	for _, r := range a.cachedResources {
		svcCounts[r.Service]++
	}
	for svc, count := range svcCounts {
		agentCtx.ResourcesByService = append(agentCtx.ResourcesByService, agent.ServiceCount{Service: svc, Count: count})
	}
	a.cachedResMu.RUnlock()
	agentCtx.IACDir = a.cfg.IACDir
	agentCtx.IACBinary = a.cfg.Terraform.Binary
	return agentCtx
}

func (a *App) listenDirChanges() {
	for newDir := range a.dirChangeCh {
		a.watcherMu.Lock()
		if a.watcher != nil {
			a.watcher.Stop()
			a.watcher = nil
		}
		a.watcherMu.Unlock()

		a.cfg.IACDir = newDir
		a.tfRunner.SetWorkDir(newDir)
		a.toolExecutor.IACDir = newDir
		a.autoDetectBinary()
		a.logf(ilog.ChannelApp, "IaC directory changed to: %s", newDir)
		a.logStartupPrerequisites()
		_ = a.stateStore.Save(&state.SessionState{
			LastProfile: a.clientManager.ActiveProfile(),
			LastIACDir:  newDir,
		})
		// Rebind Codex app-server thread cwd to the newly selected IaC directory.
		if a.cfg.LLM.Provider == "codex" {
			if err := a.applyLLMMode(a.cfg.LLM); err != nil {
				a.logf(ilog.ChannelAgent, "failed to rebind codex cwd after dir change: %v", err)
			}
		}
		go a.startWatcher()
		ctx := context.Background()
		go a.refreshDrift(ctx)
	}
}

func (a *App) listenIaCConfigChanges() {
	for newCfg := range a.iacConfigChangeCh {
		binPath, err := iac.EnsureBinary(newCfg.Binary, newCfg.Version)
		if err != nil {
			a.logf(ilog.ChannelApp, "IaC binary check: %v", err)
			a.program.Send(views.AdvisorResponseMsg{Content: fmt.Sprintf("\n\nIaC binary issue: %v", err), Done: true})
			continue
		}
		a.cfg.Terraform = newCfg
		a.tfRunner.SetBinary(binPath)
		a.toolExecutor.IACBinary = newCfg.Binary
		a.program.Send(views.AdvisorResponseMsg{Content: fmt.Sprintf("\n\nSwitched to %s (%s)", newCfg.Binary, binPath), Done: true})
		a.logf(ilog.ChannelApp, "IaC tool switched to %s (%s)", newCfg.Binary, binPath)
		ctx := context.Background()
		go a.refreshDrift(ctx)
	}
}

func (a *App) listenAWSActions() {
	for action := range a.awsActionCh {
		switch act := action.(type) {
		case tui.ProfileSelectRequest:
			if err := a.clientManager.SelectProfile(act.Profile); err != nil {
				a.logf(ilog.ChannelSystem, "profile select failed: %v", err)
				continue
			}
			_ = a.stateStore.Save(&state.SessionState{
				LastProfile: act.Profile,
				LastIACDir:  a.cfg.IACDir,
			})
			a.program.Send(tui.ProfileCatalogMsg{
				Profiles:      a.clientManager.Profiles(),
				ActiveProfile: a.clientManager.ActiveProfile(),
			})
			a.sendDashboardUpdate(time.Now())
		case tui.ProfileLoginRequest:
			a.startProfileLogin(act.Profile)
		case tui.ProfileRefreshRequest:
			if err := a.clientManager.Initialize(context.Background()); err != nil {
				a.logf(ilog.ChannelSystem, "profile refresh failed: %v", err)
				a.program.Send(views.ProfileActionStatusMsg{Text: fmt.Sprintf("Refresh failed: %v", err), IsError: true})
				continue
			}
			a.program.Send(tui.ProfileCatalogMsg{
				Profiles:      a.clientManager.Profiles(),
				ActiveProfile: a.clientManager.ActiveProfile(),
			})
			a.program.Send(views.ProfileActionStatusMsg{Text: "Profiles refreshed", IsError: false})
			a.sendDashboardUpdate(time.Now())
		case tui.ProfileCancelLoginRequest:
			if a.cancelActiveProfileLogin() {
				a.logf(ilog.ChannelSystem, "cancelled active AWS SSO login")
				a.program.Send(views.ProfileActionStatusMsg{Text: "Login canceled", IsError: false})
			} else {
				a.logf(ilog.ChannelSystem, "no active AWS SSO login to cancel")
				a.program.Send(views.ProfileActionStatusMsg{Text: "No active login to cancel", IsError: true})
			}
		}
	}
}

func (a *App) startProfileLogin(profile string) {
	if prev, cancelled := a.cancelActiveProfileLoginWithProfile(); cancelled {
		a.logf(ilog.ChannelSystem, "cancelled in-flight SSO login for profile %s", prev)
		a.program.Send(views.ProfileActionStatusMsg{Text: fmt.Sprintf("Canceled previous login (%s)", prev), IsError: false})
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.loginMu.Lock()
	a.loginSeq++
	loginID := a.loginSeq
	a.activeLoginID = loginID
	a.loginCancel = cancel
	a.loginProfile = profile
	a.loginMu.Unlock()

	a.logf(ilog.ChannelSystem, "running aws sso login --profile %s", profile)
	a.program.Send(views.ProfileActionStatusMsg{Text: fmt.Sprintf("Login started for %s", profile), IsError: false})
	go func(profile string, id uint64) {
		err := a.clientManager.LoginProfile(ctx, profile, func(line string) {
			a.logf(ilog.ChannelSystem, "%s", line)
		})

		a.loginMu.Lock()
		if a.activeLoginID == id {
			a.loginCancel = nil
			a.loginProfile = ""
			a.activeLoginID = 0
		}
		a.loginMu.Unlock()

		if err != nil {
			if ctx.Err() == context.Canceled {
				a.logf(ilog.ChannelSystem, "sso login cancelled for %s", profile)
				a.program.Send(views.ProfileActionStatusMsg{Text: fmt.Sprintf("Login canceled for %s", profile), IsError: false})
			} else {
				a.logf(ilog.ChannelSystem, "sso login failed for %s: %v", profile, err)
				a.program.Send(views.ProfileActionStatusMsg{Text: fmt.Sprintf("Login failed for %s", profile), IsError: true})
			}
		} else {
			a.logf(ilog.ChannelSystem, "sso login succeeded for %s", profile)
			a.program.Send(views.ProfileActionStatusMsg{Text: fmt.Sprintf("Login succeeded for %s", profile), IsError: false})
		}

		a.program.Send(tui.ProfileCatalogMsg{
			Profiles:      a.clientManager.Profiles(),
			ActiveProfile: a.clientManager.ActiveProfile(),
		})
		a.sendDashboardUpdate(time.Now())
		go a.refreshResources(context.Background())
	}(profile, loginID)
}

func (a *App) cancelActiveProfileLogin() bool {
	_, ok := a.cancelActiveProfileLoginWithProfile()
	return ok
}

func (a *App) cancelActiveProfileLoginWithProfile() (string, bool) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.loginCancel == nil {
		return "", false
	}
	profile := a.loginProfile
	cancel := a.loginCancel
	a.loginCancel = nil
	a.loginProfile = ""
	a.activeLoginID = 0
	cancel()
	return profile, true
}
