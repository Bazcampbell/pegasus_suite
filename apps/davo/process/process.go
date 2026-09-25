// davo/process/process.go

package process

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"pegasus_suite/apps/davo/settings"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/engine"
	"pegasus_suite/logger"
)

const app = "davo"

var aest = time.FixedZone("AEST", 10*60*60)

// Tip is one selection, named the way Betmatic names it.
type Tip struct {
	Date       string // race date, YYYY-MM-DD; "" means today
	Venue      string
	RaceNumber int
	Runner     int
	RunnerName string
	Units      float64
	Market     string // WIN or PLACE
	RatedOdds  float64
}

type Process struct {
	settings settings.ProcessSettings
	eng      *engine.Engine
	account  *engine.Account
	onClose  func()
	running  atomic.Bool
}

func New(s settings.ProcessSettings, eng *engine.Engine, account *engine.Account, onClose func()) *Process {
	return &Process{settings: s, eng: eng, account: account, onClose: onClose}
}

func (p *Process) Start()        { p.running.Store(true) }
func (p *Process) Stop()         { p.running.Store(false) }
func (p *Process) Running() bool { return p.running.Load() }

func (p *Process) Close() {
	p.Stop()
	if p.onClose != nil {
		p.onClose()
	}
}

// Arm turns the process's bookmakers on ahead of a likely bet, when it is running.
func (p *Process) Arm() {
	if !p.Running() || len(p.account.Bookmakers) == 0 {
		return
	}
	go func() {
		if err := p.eng.ArmBookies(p.account); err != nil {
			logger.Warn(logger.Log{App: app, UserID: p.settings.UserID, ProcessID: p.settings.ID, Message: fmt.Sprintf("arming bookies failed error=%v", err)})
		}
	}()
}

// Offer places t on its own goroutine when the process is running.
func (p *Process) Offer(t Tip) {
	if !p.Running() {
		return
	}
	o, err := p.order(t)
	if err != nil {
		logger.Warn(logger.Log{App: app, UserID: p.settings.UserID, ProcessID: p.settings.ID, Race: &logger.Race{Venue: t.Venue, Number: t.RaceNumber, Runner: t.Runner}, Message: err.Error()})
		return
	}
	go p.eng.Place(o)
}

// order stakes t in cash, at no less than the rated price discounted by the threshold, so a
// one-unit tip risks TargetLiability at that price.
func (p *Process) order(t Tip) (engine.Order, error) {
	minOdds := t.RatedOdds / (1 + p.settings.MinOddsThreshold/100)
	if minOdds <= 1 {
		return engine.Order{}, fmt.Errorf("min odds %.2f from rated %.2f is not bettable", minOdds, t.RatedOdds)
	}
	perUnit, units := p.settings.TargetLiability/(minOdds-1), t.Units
	if p.settings.IsTest {
		perUnit, units = 1, 1
	}

	side := engine.BetmaticWin
	if strings.EqualFold(t.Market, "place") {
		side = engine.BetmaticPlace
	}
	date := t.Date
	if date == "" {
		date = time.Now().In(aest).Format(time.DateOnly)
	}

	return engine.Order{
		Account: p.account,
		Race:    engine.Race{Date: date, Venue: t.Venue, Code: betmatic.THOROUGHBRED, Number: t.RaceNumber},
		Runner:  t.Runner,
		Side:    side,
		Unit:    units,
		Stake:   engine.Stake{Betmatic: engine.BetmaticStake{WinStake: perUnit, MinOdds: minOdds, Cash: true}},
	}, nil
}
