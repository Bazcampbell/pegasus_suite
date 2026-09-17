// exchange/order.go

package exchange

import (
	"fmt"
	"strings"
)

// ExecutionError is a bet Betfair declined. placeOrders answers 200 OK with the
// rejection in the body, so without this a bare error check reads failure as
// success.
type ExecutionError struct {
	Endpoint          string
	Status            ExecutionReportStatus
	ErrorCode         ExecutionReportErrorCode
	InstructionErrors []string
}

func (e *ExecutionError) Error() string {
	msg := fmt.Sprintf("betfair %s: %s", e.Endpoint, e.Status)
	if e.ErrorCode != "" {
		msg += ": " + string(e.ErrorCode)
	}
	if len(e.InstructionErrors) > 0 {
		msg += " (" + strings.Join(e.InstructionErrors, ", ") + ")"
	}
	return msg
}

func (c *Client) PlaceOrders(req PlaceOrdersRequest) (PlaceExecutionReport, error) {
	report, err := post[PlaceExecutionReport](c, "placeOrders", req)
	if err != nil {
		return report, err
	}

	if report.Status == ExecutionReportStatusSuccess {
		return report, nil
	}

	var instructionErrors []string
	for _, r := range report.InstructionReports {
		if r.Status != InstructionReportStatusSuccess {
			instructionErrors = append(instructionErrors, string(r.ErrorCode))
		}
	}

	return report, &ExecutionError{
		Endpoint:          "placeOrders",
		Status:            report.Status,
		ErrorCode:         report.ErrorCode,
		InstructionErrors: instructionErrors,
	}
}
