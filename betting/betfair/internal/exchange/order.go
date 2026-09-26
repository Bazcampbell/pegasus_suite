// exchange/order.go

package exchange

import (
	"fmt"
	"strings"

	"github.com/Bazcampbell/goreq"
)

// rejected bet payload, still returns 200
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
	report, err := goreq.PostType[PlaceExecutionReport](bettingURL+"/placeOrders/", req, withToken(c.orderOptions, c.SessionToken()))
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
