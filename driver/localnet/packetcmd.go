package localnet

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
)

type Cmd string

const (
	CmdConnect      Cmd = "conn"
	CmdDisconnect   Cmd = "disc"
	CmdOpenLogical  Cmd = "opch"
	CmdCloseLogical Cmd = "clch"
	CmdTransmit     Cmd = "tran"
	CmdResponse     Cmd = "resp"
)

type IPacketCmd interface {
	GetCmd() Cmd
	GetErr() string
}

type IPacketBody interface {
	IPacketCmd
	GetBody() []byte
}

type IPacketConnect interface {
	IPacketCmd
	GetDevice() string
	GetProto() string
	GetSlot() uint8
}

type PacketCmd struct {
	Cmd Cmd
	Err string
}

type PacketBody struct {
	PacketCmd
	Body []byte
}

type PacketConnect struct {
	PacketCmd
	Device string
	Proto  string
	Slot   uint8
}

func init() {
	gob.Register(&PacketCmd{})
	gob.Register(&PacketBody{})
	gob.Register(&PacketConnect{})
}

func Decode(byteArray []byte) (p IPacketCmd, e error) {
	gr, err := gzip.NewReader(bytes.NewReader(byteArray))
	if err != nil {
		return nil, fmt.Errorf("decode, reader error using gzip: %w", err)
	}
	defer gr.Close()

	dec := gob.NewDecoder(gr)
	e = dec.Decode(&p)
	return p, e
}

func Encode(p IPacketCmd) (byteArray []byte, err error) {
	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)

	enc := gob.NewEncoder(gw)
	if err = enc.Encode(&p); err != nil {
		return nil, fmt.Errorf("encode, writer error using gzip: %w", err)
	}

	if err = gw.Close(); err != nil {
		return nil, fmt.Errorf("encode, error closing gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

func (p PacketCmd) GetCmd() Cmd {
	return p.Cmd
}

func (p PacketCmd) GetErr() string {
	return p.Err
}

func (p PacketBody) GetBody() []byte {
	return p.Body
}

func (p PacketConnect) GetDevice() string {
	return p.Device
}

func (p PacketConnect) GetProto() string {
	return p.Proto
}

func (p PacketConnect) GetSlot() uint8 {
	return p.Slot
}

func (p PacketCmd) String() string {
	if p.GetErr() == "" {
		return fmt.Sprintf("Cmd: %s", p.GetCmd())
	} else {
		return fmt.Sprintf("Cmd: %s, Err: %s", p.GetCmd(), p.GetErr())
	}
}

func (p PacketBody) String() string {
	return fmt.Sprintf("%s, Body(size): %4d, Body(hex): %X", p.PacketCmd, len(p.GetBody()), p.GetBody())
}

func (p PacketConnect) String() string {
	return fmt.Sprintf("%s, Device: %s, Proto: %s, Slot: %d", p.PacketCmd, p.GetDevice(), p.GetProto(), p.GetSlot())
}

func NewPacketCmd(cmd Cmd) IPacketCmd {
	return PacketCmd{cmd, ""}
}

func NewPacketCmdErr(cmd Cmd, err string) IPacketCmd {
	return PacketCmd{cmd, err}
}

func NewPacketBody(cmd Cmd, body []byte) IPacketCmd {
	return PacketBody{PacketCmd{cmd, ""}, body}
}

func NewPacketConnect(device string, proto string, slot uint8) IPacketCmd {
	return PacketConnect{PacketCmd{CmdConnect, ""}, device, proto, slot}
}
