# eUICC Go Module

A Go implementation of eUICC (Embedded Universal Integrated Circuit Card) profile management protocol with UDP network bridge capabilities.

## 🎯 Overview

This project provides a **UDP-based bridge server** that enables remote access to eUICC/eSIM devices through various hardware interfaces (AT, MBIM, QMI). It acts as a network proxy between client applications and physical eUICC chips, allowing eUICC profile management operations over the network.

### Key Features

- 🌐 **UDP Network Bridge**: Remote access to eUICC devices via UDP protocol
- 🔌 **Multiple Protocol Support**: AT commands, MBIM, QMI, and QRTR interfaces
- 📦 **Compressed Communication**: GZIP-compressed GOB encoding for efficient data transfer
- 🔒 **Thread-Safe Operations**: Concurrent request handling with mutex protection
- 🛡️ **Error Handling**: Comprehensive error reporting and validation
- 📊 **Debug Logging**: Structured logging with slog for troubleshooting

## 🚀 Quick Start

### Prerequisites

- Go 1.21 or higher
- Access to an eUICC-enabled device
- Appropriate drivers (AT/MBIM/QMI) for your hardware

### Installation
```bash
git clone https://github.com/avwarez/euicc-go-module.git
cd euicc-go-module
go mod download
```

### Running the Server
```bash
# Basic usage with defaults (0.0.0.0:8080)
go run server/main.go

# Custom configuration
go run server/main.go -bindAddr 127.0.0.1 -bindPort 9000 -bufferSize 4096
```

### Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-bindAddr` | `0.0.0.0` | Server binding address |
| `-bindPort` | `8080` | Server listening port |
| `-bufferSize` | `2048` | UDP buffer size in bytes |

## 📡 Protocol Documentation

### Packet Structure

All packets are compressed using GZIP and encoded with GOB. The protocol supports the following commands:

#### Command Types

| Command | Code | Description |
|---------|------|-------------|
| Connect | `conn` | Establish connection to eUICC device |
| Disconnect | `disc` | Close connection to eUICC device |
| Open Logical Channel | `opch` | Open a logical channel with AID |
| Close Logical Channel | `clch` | Close a logical channel |
| Transmit APDU | `tran` | Send APDU command to eUICC |
| Response | `resp` | Server response to client |

## 🔧 Supported Hardware Protocols

### AT Commands (`at`)
- Standard Hayes AT command interface
- Common in USB modems and cellular modules
- Device example: `/dev/ttyUSB0`, `/dev/ttyACM0`

### MBIM (`mbim`)
- Mobile Broadband Interface Model
- Used in modern LTE/5G modems
- Device example: `/dev/cdc-wdm0`

### QMI (`qmi`)
- Qualcomm MSM Interface
- Qualcomm-specific protocol
- Device example: `/dev/cdc-wdm0`

### QRTR (`qrtr`)
- Qualcomm IPC Router
- For devices with QRTR support
- No device path needed (uses slot number only)

## 🛠️ Development

### Project Structure
```
euicc-go-module/
├── server/
│   └── main.go                # Server entry point
├── driver/
│   └── localnet/
│       ├── packetcmd.go      # Packet definitions and encoding
│       └── simpleudp.go      # UDP client implementation
└── examples/                  # Usage examples
```

### Building
```bash
# Build for current platform
go build -o euicc-server server/main.go

# Build for Linux ARM64 (e.g., Raspberry Pi)
GOOS=linux GOARCH=arm64 go build -o euicc-server-arm64 server/main.go

# Build for Linux x86_64
GOOS=linux GOARCH=amd64 go build -o euicc-server-amd64 server/main.go
```

## 🔒 Security Considerations

- **Network Exposure**: The server listens on all interfaces by default. Use `-bindAddr 127.0.0.1` for local-only access
- **No Authentication**: Currently no authentication mechanism. Use firewall rules or SSH tunneling for production
- **Single Connection**: Server handles one eUICC connection at a time
- **Buffer Limits**: Default 2KB buffer, increase for large APDU commands

## 📚 References

- [GSMA SGP.22 v2.5](https://www.gsma.com/esim/wp-content/uploads/2020/06/SGP.22-v2.5.pdf) - RSP Technical Specification
- [damonto/euicc-go](https://github.com/damonto/euicc-go) - Base LPA implementation
- [ETSI TS 102 221](https://www.etsi.org/deliver/etsi_ts/102200_102299/102221/) - Smart card UICC-Terminal interface

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [damonto/euicc-go](https://github.com/damonto/euicc-go) for the base eUICC implementation
- GSMA for the SGP.22 specification
- The Go community for excellent networking libraries

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/avwarez/euicc-go-module/issues)
- **Discussions**: [GitHub Discussions](https://github.com/avwarez/euicc-go-module/discussions)

---

**Note**: This is bridge server software. You still need a complete LPA (Local Profile Assistant) implementation to perform full eUICC profile management operations.
