// engine/processes.go

package engine

import (
	"fmt"
	"pegasus_suite/apps/davo/tenant"

	betengine "github.com/Bazcampbell/bazbet-sdk/engine"
	"pegasus_suite/betting"
	logger "pegasus_suite/logger"
)

func (e *Engine) AddProcess(userID, processID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.processes[userID][processID]; ok {
		return fmt.Errorf("process already added")
	}

	processSettings, err := e.db.GetProcessSettings(userID, processID)
	if err != nil {
		return fmt.Errorf("unable to get process settings: %w", err)
	}

	if err := betengine.AddBetmaticClient(processSettings.BetmaticEmail, processSettings.BetmaticPassword, processID); err != nil {
		return fmt.Errorf("unable to add betmatic client: %w", err)
	}

	process, err := tenant.NewProcess(*processSettings)
	if err != nil {
		return err
	}

	if e.processes[userID] == nil {
		e.processes[userID] = make(map[string]*tenant.Process)
	}
	e.processes[userID][processID] = process

	logger.Debug(logger.InfoLog{
		Message:   "added process",
		UserID:    userID,
		ProcessID: processID,
	})

	return nil
}

func (e *Engine) GetProcess(userId, id string) (*tenant.Process, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	p, ok := e.processes[userId][id]
	if !ok {
		return nil, fmt.Errorf("process not found")
	}
	return p, nil
}

func (e *Engine) RestartProcess(userID, processID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	old, ok := e.processes[userID][processID]
	if !ok {
		return fmt.Errorf("process not found")
	}

	// called on settings saved on active process, reload them
	processSettings, err := e.db.GetProcessSettings(userID, processID)
	if err != nil {
		return fmt.Errorf("unable to get process settings: %w", err)
	}

	old.Stop()

	if err := betengine.AddBetmaticClient(processSettings.BetmaticEmail, processSettings.BetmaticPassword, processID); err != nil {
		return fmt.Errorf("unable to add betmatic client: %w", err)
	}

	newProc, err := tenant.NewProcess(*processSettings)
	if err != nil {
		return err
	}

	e.processes[userID][processID] = newProc

	releaseIfChanged(old.Settings.BetmaticEmail, processSettings.BetmaticEmail, userID, processID)

	newProc.Start()

	logger.Debug(logger.InfoLog{
		Message:   "restarting process",
		UserID:    userID,
		ProcessID: processID,
	})

	return nil
}

func (e *Engine) StartProcess(userID, processID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	process, ok := e.processes[userID][processID]
	if !ok {
		return fmt.Errorf("process not found")
	}
	process.Start()

	logger.Info(logger.InfoLog{
		Message:   "started process",
		UserID:    userID,
		ProcessID: processID,
	})

	return nil
}

func (e *Engine) StopProcess(userID, processID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	process, ok := e.processes[userID][processID]
	if !ok {
		return fmt.Errorf("process not found")
	}
	process.Stop()

	logger.Debug(logger.InfoLog{
		Message:   "stopped process",
		UserID:    userID,
		ProcessID: processID,
	})

	return nil
}

func (e *Engine) DeleteProcess(userID, processID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	proc, ok := e.processes[userID][processID]
	if !ok {
		return fmt.Errorf("process not found")
	}

	proc.Stop()
	releaseIfChanged(proc.Settings.BetmaticEmail, "", userID, processID)

	delete(e.processes[userID], processID)

	logger.Debug(logger.InfoLog{
		Message:   "deleted process",
		UserID:    userID,
		ProcessID: processID,
	})

	return nil
}

func releaseIfChanged(old, current, userID, processID string) {
	if old == "" || old == current {
		return
	}

	betengine.RemoveClient(old, betting.ProviderBetmatic, processID)
}
