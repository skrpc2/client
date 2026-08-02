package rycli

import (
	"errors"
	"fmt"
)

// encodeClientRequest encodes parameters for a JSON-RPC client request.
func encodeClientRequest(method string, args interface{}) ([]byte, error) {
	return marshalPayload(args)
}

// decodeClientResponse decodes the response body of a client request into the interface reply.
func decodeClientResponse(method string, r []byte, result interface{}) error {
	arg, err := unmarshalEnvelope(r)
	if err != nil {
		return err
	}

	if len(arg.Err) > 0 {
		return errors.New(arg.Err)
	}

	if err := unmarshalPayload(arg.Data, result); err != nil {
		return fmt.Errorf("rpc call %s() on could not decode body to rpc Decode: %s", method, err.Error())
	}

	return nil
}
