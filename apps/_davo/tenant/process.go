// tenant/process.go

package tenant

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"pegasus_suite/apps/davo/core"
	"pegasus_suite/apps/davo/store"
	"strings"
	"sync"

	betengine "github.com/Bazcampbell/bazbet-sdk/engine"
	logger "pegasus_suite/logger"
)

// runtime container for one user's bot instance. Lifecycle:
//
//	NewProcess()  → stopped (Status() == false)
//	Start()       → spawns run() in a goroutine, Status() == true
//	Stop()        → cancels the context, run() returns, Status() == false
//	Start() again → recreates ctx/cancel and re-spawns run()
//
// All ctx/cancel mutation is mu-guarded so concurrent Status/Start/Stop
// calls (e.g. the /status poll racing with /start) stay consistent.
//
// It holds no betmatic client: the session lives in the ENGINE service, which
// this process claims by adding its account and reaches by process id.
type Process struct {
	Settings store.ProcessSettings

	RaceDataInbox chan core.DavoRaceMessage

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

func NewProcess(settings store.ProcessSettings) (*Process, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // start stopped

	err := validateSettings(settings)
	if err != nil {
		return nil, err
	}

	p := &Process{
		Settings:      settings,
		RaceDataInbox: make(chan core.DavoRaceMessage, 5),
		ctx:           ctx,
		cancel:        cancel,
	}

	return p, nil
}

// replace cancelled ctx with fresh
// each Start() gets own ctx + cancel so Stop() only cancels that run
func (p *Process) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx.Err() == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.ctx = ctx
	p.cancel = cancel

	logger.Debug(logger.InfoLog{
		Message:   "starting process",
		ProcessID: p.Settings.ID,
		UserID:    p.Settings.UserID,
	})

	go p.run(ctx)
}

func (p *Process) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx.Err() != nil {
		return
	}

	logger.Debug(logger.InfoLog{
		Message:   "stopping process",
		ProcessID: p.Settings.ID,
		UserID:    p.Settings.UserID,
	})
	p.cancel()
}

func (p *Process) Status() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ctx.Err() == nil
}

// arm bookies
func (p *Process) OnPreBetMessage() {
	if len(p.Settings.Bookmakers) == 0 {
		return
	}

	req := map[string]any{"bookmakers": p.Settings.Bookmakers, "bot_id": p.Settings.BotID}

	if err := betengine.ArmBetmaticBookies(p.Settings.BetmaticEmail, p.Settings.ID, p.Settings.BotID, p.Settings.Bookmakers); err != nil {
		logger.Warn(logger.ErrorLog{
			Message:   fmt.Sprintf("failed to arm bookies error=%v", err),
			UserID:    p.Settings.UserID,
			ProcessID: p.Settings.ID,
			Request:   req,
		})
		return
	}

	logger.Debug(logger.InfoLog{
		Message:   fmt.Sprintf("successfully armed bookies request=%+v", req),
		UserID:    p.Settings.UserID,
		ProcessID: p.Settings.ID,
	})
}

func validateSettings(s store.ProcessSettings) error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("id is required")
	}

	if strings.TrimSpace(s.UserID) == "" {
		return errors.New("user id is required")
	}

	if strings.TrimSpace(s.BetmaticEmail) == "" {
		return errors.New("betmatic email is required")
	}

	if _, err := mail.ParseAddress(s.BetmaticEmail); err != nil {
		return errors.New("betmatic email is invalid")
	}

	if strings.TrimSpace(s.BetmaticPassword) == "" {
		return errors.New("betmatic password is required")
	}

	// odds validation
	if s.MinOdds != 0 && s.MaxOdds != 0 {
		if s.MaxOdds <= s.MinOdds {
			return fmt.Errorf("win max odds (%.2f) must be greater than win min odds (%.2f)", s.MaxOdds, s.MinOdds)
		}
	}

	return nil
}
