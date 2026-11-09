package worker

import (
	"context"
	"encoding/json"
	"fmt"
)

type EmailPayload struct {
	To              string `json:"to"`
	SimulateFailure bool   `json:"simulate_failure"`
}

func Execute(
	ctx context.Context,
	taskType string,
	payload json.RawMessage,
) error {

	if ctx.Err() != nil {
		return ctx.Err()
	}
	switch taskType {
	case "email":
		var p EmailPayload
		err := json.Unmarshal(payload, &p)
		if err != nil {
			return err
		}
		if p.SimulateFailure == true {
			return fmt.Errorf("SimulateFailure == true")
		}
		return nil

	default:
		return fmt.Errorf("no support tasktype")
	}
}
