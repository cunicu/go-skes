// SPDX-FileCopyrightText: 2023 Joern Barthel
// SPDX-FileCopyrightText: 2023 Steffen Vogel <post@steffenvogel.de>
// SPDX-License-Identifier: Apache-2.0

// package yk implements the Yubico OATH protocol
// for the Yubikey OATH-HOTP/TOTP hardware tokens
// https://developers.yubico.com/OATH/YKOATH_Protocol.html
package yk

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"cunicu.li/hawkes/internal/iso7816"
	"github.com/ebfe/scard"
)

type (
	tag         byte
	instruction byte
)

const HMACMinimumKeySize = 14

// TLV tags for credential data
const (
	tagName      tag = 0x71
	tagNameList  tag = 0x72
	tagKey       tag = 0x73
	tagChallenge tag = 0x74
	tagResponse  tag = 0x75
	tagTruncated tag = 0x76
	tagHOTP      tag = 0x77
	tagProperty  tag = 0x78
	tagVersion   tag = 0x79
	tagImf       tag = 0x7A
	tagAlgorithm tag = 0x7B
	tagTouch     tag = 0x7C
)

// Instruction bytes for commands
const (
	insList          instruction = 0xA1
	insSelect        instruction = 0xA4
	insPut           instruction = 0x01
	insDelete        instruction = 0x02
	insSetCode       instruction = 0x03
	insReset         instruction = 0x04
	insRename        instruction = 0x05
	insCalculate     instruction = 0xA2
	insValidate      instruction = 0xA3
	insCalculateAll  instruction = 0xA4
	insSendRemaining instruction = 0xA5
)

// OATH implements most parts of the TOTP portion of the YKOATH specification
// https://developers.yubico.com/OATH/YKOATH_Protocol.html
type OATH struct {
	Clock  func() time.Time
	Period time.Duration

	card    *scard.Card
	context *scard.Context
}

var (
	errFailedToConnect            = errors.New("failed to connect to reader")
	errFailedToDisconnect         = errors.New("failed to disconnect from reader")
	errFailedToEstablishContext   = errors.New("failed to establish context")
	errFailedToListReaders        = errors.New("failed to list readers")
	errFailedToListSuitableReader = errors.New("no suitable reader found")
	errFailedToReleaseContext     = errors.New("failed to release context")
	errFailedToTransmit           = errors.New("failed to transmit APDU")
	errUnknownTag                 = errors.New("unknown tag")
)

// New initializes a new OATH session
func New() (*OATH, error) {
	context, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToEstablishContext, err)
	}

	readers, err := context.ListReaders()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToListReaders, err)
	}

	for _, reader := range readers {
		if strings.Contains(strings.ToLower(reader), "yubikey") {
			card, err := context.Connect(reader, scard.ShareShared, scard.ProtocolAny)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", errFailedToConnect, err)
			}

			return &OATH{
				Clock:   time.Now,
				Period:  30 * time.Second,
				card:    card,
				context: context,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w (out of %d readers)", errFailedToListSuitableReader, len(readers))
}

// Close terminates an OATH session
func (o *OATH) Close() error {
	if err := o.card.Disconnect(scard.LeaveCard); err != nil {
		return fmt.Errorf("%w: %w", errFailedToDisconnect, err)
	}

	if err := o.context.Release(); err != nil {
		return fmt.Errorf("%w: %w", errFailedToReleaseContext, err)
	}

	return nil
}

// send sends an APDU to the card
func (o *OATH) send(cla byte, ins instruction, p1, p2 byte, data ...[]byte) (tvs, error) { //nolint:unparam
	var (
		code    iso7816.Code
		results []byte
		send    = append([]byte{cla, byte(ins), p1, p2}, write(0x00, data...)...)
	)

	for {
		res, err := o.card.Transmit(send)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errFailedToTransmit, err)
		}

		code = res[len(res)-2:]
		results = append(results, res[0:len(res)-2]...)

		switch {
		case code.IsMore():
			send = []byte{0x00, 0xa5, 0x00, 0x00}
		case code.IsSuccess():
			return read(results), nil
		default:
			return nil, code
		}
	}
}