/*
 * @Author: ph4ntom
 * @Date: 2021-03-10 15:27:30
 * @LastEditors: ph4ntom
 * @LastEditTime: 2021-03-26 16:53:54
 */

package process

import (
	"Stowaway/agent/handler"
	"Stowaway/agent/initial"
	"Stowaway/agent/manager"
	"Stowaway/crypto"
	"Stowaway/protocol"
	"Stowaway/share"
	"Stowaway/utils"
	"log"
	"net"
	"os"
)

type Agent struct {
	UUID         string
	Conn         net.Conn
	Memo         string
	CryptoSecret []byte
	UserOptions  *initial.Options
}

func NewAgent(options *initial.Options) *Agent {
	agent := new(Agent)
	agent.UUID = protocol.TEMP_UUID
	agent.CryptoSecret, _ = crypto.KeyPadding([]byte(options.Secret))
	agent.UserOptions = options
	return agent
}

func (agent *Agent) Run() {
	agent.sendMyInfo()
	agent.handleDataFromUpstream()
	//agent.handleDataFromDownstream()
}

func (agent *Agent) sendMyInfo() {
	sMessage := protocol.PrepareAndDecideWhichSProtoToUpper(agent.Conn, agent.UserOptions.Secret, agent.UUID)
	header := &protocol.Header{
		Sender:      agent.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.MYINFO,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))), // No need to set route when agent send mess to admin
		Route:       protocol.TEMP_ROUTE,
	}

	hostname, username := utils.GetSystemInfo()

	myInfoMess := &protocol.MyInfo{
		UsernameLen: uint64(len(username)),
		Username:    username,
		HostnameLen: uint64(len(hostname)),
		Hostname:    hostname,
	}

	protocol.ConstructMessage(sMessage, header, myInfoMess)
	sMessage.SendMessage()
}

func (agent *Agent) handleDataFromUpstream() {
	rMessage := protocol.PrepareAndDecideWhichRProtoFromUpper(agent.Conn, agent.UserOptions.Secret, agent.UUID)
	//sMessage := protocol.PrepareAndDecideWhichSProtoToUpper(agent.Conn, agent.UserOptions.Secret, agent.ID)
	component := &protocol.MessageComponent{
		Secret: agent.UserOptions.Secret,
		Conn:   agent.Conn,
		UUID:   agent.UUID,
	}

	var socks *handler.Socks
	shell := handler.NewShell()
	mySSH := handler.NewSSH()
	mgr := manager.NewManager(share.NewFile())

	for {
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)
		if err != nil {
			log.Println("[*]Peer node seems offline!")
			break
		}
		if fHeader.Accepter == agent.UUID {
			switch fHeader.MessageType {
			case protocol.MYMEMO:
				message := fMessage.(*protocol.MyMemo)
				agent.Memo = message.Memo
			case protocol.SHELLREQ:
				// No need to check member "start"
				go shell.Start(component)
			case protocol.SHELLCOMMAND:
				message := fMessage.(*protocol.ShellCommand)
				shell.Input(message.Command)
			case protocol.LISTENREQ:
				//message := fMessage.(*protocol.ListenReq)
				//go handler.StartListen(message.Addr)
			case protocol.SSHREQ:
				message := fMessage.(*protocol.SSHReq)
				mySSH.Addr = message.Addr
				mySSH.Method = int(message.Method)
				mySSH.Username = message.Username
				mySSH.Password = message.Password
				mySSH.Certificate = message.Certificate
				go mySSH.Start(component)
			case protocol.SSHCOMMAND:
				message := fMessage.(*protocol.SSHCommand)
				mySSH.Input(message.Command)
			case protocol.FILESTATREQ:
				message := fMessage.(*protocol.FileStatReq)
				mgr.File.FileName = message.Filename
				mgr.File.SliceNum = message.SliceNum
				err := mgr.File.CheckFileStat(component, protocol.TEMP_ROUTE, protocol.ADMIN_UUID, share.AGENT)
				if err == nil {
					go mgr.File.Receive(component, protocol.TEMP_ROUTE, protocol.ADMIN_UUID, share.AGENT)
				}
			case protocol.FILESTATRES:
				message := fMessage.(*protocol.FileStatRes)
				if message.OK == 1 {
					go mgr.File.Upload(component, protocol.TEMP_ROUTE, protocol.ADMIN_UUID, share.AGENT)
				} else {
					mgr.File.Handler.Close()
				}
			case protocol.FILEDATA:
				message := fMessage.(*protocol.FileData)
				mgr.File.DataChan <- message.Data
			case protocol.FILEERR:
				// No need to check message
				mgr.File.ErrChan <- true
			case protocol.FILEDOWNREQ:
				message := fMessage.(*protocol.FileDownReq)
				mgr.File.FilePath = message.FilePath
				mgr.File.FileName = message.Filename
				mgr.File.SendFileStat(component, protocol.TEMP_ROUTE, protocol.ADMIN_UUID, share.AGENT)
			case protocol.SOCKSSTART:
				message := fMessage.(*protocol.SocksStart)
				socks = handler.NewSocks(message.Username, message.Password)
				go socks.Start(mgr, component)
			case protocol.SOCKSDATA:
				// message := fMessage.(*protocol.SocksData)
				// handler.
			case protocol.OFFLINE:
				// No need to check message
				os.Exit(0)
			default:
				log.Println("[*]Unknown Message!")
			}
		}
	}
}