/*
 * Copyright 2018-2023 Open Networking Foundation (ONF) and the ONF Contributors

 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package devices

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

  "encoding/json"
//  "io/ioutil"
  "os"
//  "bytes"
  "bufio"

	"github.com/opencord/voltha-protos/v5/go/extension"

	"github.com/opencord/bbsim/internal/bbsim/responders/dhcp"
	"github.com/opencord/bbsim/internal/bbsim/types"
	omcilib "github.com/opencord/bbsim/internal/common/omci"
	"github.com/opencord/voltha-protos/v5/go/ext/config"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/looplab/fsm"
	"github.com/opencord/bbsim/internal/bbsim/packetHandlers"
	"github.com/opencord/bbsim/internal/common"
	"github.com/opencord/voltha-protos/v5/go/openolt"
	"github.com/opencord/voltha-protos/v5/go/tech_profile"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
  "github.com/opencord/voltha-protos/v5/go/bossopenolt"
)

var oltLogger = log.WithFields(log.Fields{
	"module": "OLT",
})

const (
	//InternalState FSM states and transitions
	OltInternalStateCreated     = "created"
	OltInternalStateInitialized = "initialized"
	OltInternalStateEnabled     = "enabled"
	OltInternalStateDisabled    = "disabled"
	OltInternalStateDeleted     = "deleted"

	OltInternalTxInitialize = "initialize"
	OltInternalTxEnable     = "enable"
	OltInternalTxDisable    = "disable"
	OltInternalTxDelete     = "delete"
)

type OltDevice struct {
	sync.Mutex
	OltServer *grpc.Server

	// BBSIM Internals
	ID                   int
	SerialNumber         string
	NumNni               int
	NniSpeed             uint32
	NumPon               int
	NumOnuPerPon         int
	NumUni               int
	NumPots              int
	NniDhcpTrapVid       int
	InternalState        *fsm.FSM
	channel              chan types.Message
	dhcpServer           dhcp.DHCPServerIf
	Flows                sync.Map
	Delay                int
	ControlledActivation mode
	EventChannel         chan common.Event
	PublishEvents        bool
	PortStatsInterval    int
	PreviouslyConnected  bool

	Pons []*PonPort
	Nnis []*NniPort

	// OLT Attributes
	OperState *fsm.FSM

	enableContext       context.Context
	enableContextCancel context.CancelFunc

	OpenoltStream openolt.Openolt_EnableIndicationServer
	enablePerf    bool

	// Allocated Resources
	// this data are to verify that the openolt adapter does not duplicate resources
	AllocIDsLock     sync.RWMutex
	AllocIDs         map[uint32]map[uint32]map[uint32]map[int32]map[uint64]bool // map[ponPortId]map[OnuId]map[PortNo]map[AllocIds]map[FlowId]bool
	GemPortIDsLock   sync.RWMutex
	GemPortIDs       map[uint32]map[uint32]map[uint32]map[int32]map[uint64]bool // map[ponPortId]map[OnuId]map[PortNo]map[GemPortIDs]map[FlowId]bool
	OmciResponseRate uint8
	signature        uint32
  OltStats         []openolt.PortStatistics
}

var olt OltDevice
var latencyFlag bool = false

func GetOLT() *OltDevice {
	return &olt
}

func CreateOLT(options common.GlobalConfig, services []common.ServiceYaml, isMock bool) *OltDevice {
	oltLogger.WithFields(log.Fields{
		"ID":             options.Olt.ID,
		"NumNni":         options.Olt.NniPorts,
		"NniSpeed":       options.Olt.NniSpeed,
		"NumPon":         options.Olt.PonPorts,
		"NumOnuPerPon":   options.Olt.OnusPonPort,
		"NumUni":         options.Olt.UniPorts,
		"NumPots":        options.Olt.PotsPorts,
		"NniDhcpTrapVid": options.Olt.NniDhcpTrapVid,
	}).Debug("CreateOLT")

	olt = OltDevice{
		ID:           options.Olt.ID,
		SerialNumber: fmt.Sprintf("ETRI_OLT_%d", options.Olt.ID),
		OperState: getOperStateFSM(func(e *fsm.Event) {
			oltLogger.Debugf("Changing OLT OperState from %s to %s", e.Src, e.Dst)
		}),
		NumNni:              int(options.Olt.NniPorts),
		NniSpeed:            options.Olt.NniSpeed,
		NumPon:              int(options.Olt.PonPorts),
		NumOnuPerPon:        int(options.Olt.OnusPonPort),
		NumUni:              int(options.Olt.UniPorts),
		NumPots:             int(options.Olt.PotsPorts),
		NniDhcpTrapVid:      int(options.Olt.NniDhcpTrapVid),
		Pons:                []*PonPort{},
		Nnis:                []*NniPort{},
		Delay:               options.BBSim.Delay,
		enablePerf:          options.BBSim.EnablePerf,
		PublishEvents:       options.BBSim.Events,
		PortStatsInterval:   options.Olt.PortStatsInterval,
		dhcpServer:          dhcp.NewDHCPServer(),
		PreviouslyConnected: false,
		AllocIDs:            make(map[uint32]map[uint32]map[uint32]map[int32]map[uint64]bool),
		GemPortIDs:          make(map[uint32]map[uint32]map[uint32]map[int32]map[uint64]bool),
		OmciResponseRate:    options.Olt.OmciResponseRate,
		signature:           uint32(time.Now().Unix()),
	}

	if val, ok := ControlledActivationModes[options.BBSim.ControlledActivation]; ok {
		olt.ControlledActivation = val
	} else {
		// FIXME throw an error if the ControlledActivation is not valid
		oltLogger.Warn("Unknown ControlledActivation Mode given, running in Default mode")
		olt.ControlledActivation = Default
	}

	// OLT State machine
	// NOTE do we need 2 state machines for the OLT? (InternalState and OperState)
	olt.InternalState = fsm.NewFSM(
		OltInternalStateCreated,
		fsm.Events{
			{Name: OltInternalTxInitialize, Src: []string{OltInternalStateCreated, OltInternalStateDeleted}, Dst: OltInternalStateInitialized},
			{Name: OltInternalTxEnable, Src: []string{OltInternalStateInitialized, OltInternalStateDisabled}, Dst: OltInternalStateEnabled},
			{Name: OltInternalTxDisable, Src: []string{OltInternalStateEnabled}, Dst: OltInternalStateDisabled},
			// delete event in enabled state below is for reboot OLT case.
			{Name: OltInternalTxDelete, Src: []string{OltInternalStateDisabled, OltInternalStateEnabled}, Dst: OltInternalStateDeleted},
		},
		fsm.Callbacks{
			"enter_state": func(e *fsm.Event) {
				oltLogger.Debugf("Changing OLT InternalState from %s to %s", e.Src, e.Dst)
			},
			fmt.Sprintf("enter_%s", OltInternalStateInitialized): func(e *fsm.Event) { olt.InitOlt() },
			fmt.Sprintf("enter_%s", OltInternalStateDeleted): func(e *fsm.Event) {
				// remove all the resource allocations
				olt.clearAllResources()
			},
		},
	)

	if !isMock {
		// create NNI Port
		nniPort, err := CreateNNI(&olt)
		if err != nil {
			oltLogger.Fatalf("Couldn't create NNI Port: %v", err)
		}

		olt.Nnis = append(olt.Nnis, &nniPort)
	}

	// Create device and Services
	nextCtag := map[string]int{}
	nextStag := map[string]int{}

	// create PON ports
	for i := 0; i < olt.NumPon; i++ {
		ponConf, err := common.GetPonConfigById(uint32(i))
		if err != nil {
			oltLogger.WithFields(log.Fields{
				"Err":    err,
				"IntfId": i,
			}).Fatal("cannot-get-pon-configuration")
		}

		tech, err := common.PonTechnologyFromString(ponConf.Technology)
		if err != nil {
			oltLogger.WithFields(log.Fields{
				"Err":    err,
				"IntfId": i,
			}).Fatal("unkown-pon-port-technology")
		}

		// initialize the resource maps for every PON Ports
		olt.AllocIDs[uint32(i)] = make(map[uint32]map[uint32]map[int32]map[uint64]bool)
		olt.GemPortIDs[uint32(i)] = make(map[uint32]map[uint32]map[int32]map[uint64]bool)

		p := CreatePonPort(&olt, uint32(i), tech)

		// create ONU devices
		if (ponConf.OnuRange.EndId - ponConf.OnuRange.StartId + 1) < uint32(olt.NumOnuPerPon) {
			oltLogger.WithFields(log.Fields{
				"OnuRange":     ponConf.OnuRange,
				"RangeSize":    ponConf.OnuRange.EndId - ponConf.OnuRange.StartId + 1,
				"NumOnuPerPon": olt.NumOnuPerPon,
				"IntfId":       i,
			}).Fatal("onus-per-pon-bigger-than-resource-range-size")
		}

		for j := 0; j < olt.NumOnuPerPon; j++ {
			delay := time.Duration(olt.Delay*j) * time.Millisecond
			o := CreateONU(&olt, p, uint32(j+1), delay, nextCtag, nextStag, isMock)

			p.Onus = append(p.Onus, o)
		}
		olt.Pons = append(olt.Pons, p)
	}

	if !isMock {
		if err := olt.InternalState.Event(OltInternalTxInitialize); err != nil {
			log.Errorf("Error initializing OLT: %v", err)
			return nil
		}
	}

	if olt.PublishEvents {
		log.Debugf("BBSim event publishing is enabled")
		// Create a channel to write event messages
		olt.EventChannel = make(chan common.Event, 100)
	}
  InitOltStats(&olt)
	return &olt
}

func InitOltStats(olt *OltDevice){

  filePath := "./olt_stats.txt"

  file, err := os.Open(filePath)

  if err!=nil {
      oltLogger.WithFields(log.Fields{
        "Error": err,
      }).Fatal("Can not Open File")
  }
  defer file.Close()

  content := bufio.NewScanner(file)

  content.Split(bufio.ScanLines)
//  for _, line := range lines{
//    var data openolt.PortStatistics
//    err:= json.Unmarshal(line, &data)
//
//    if err !=nil {
//        oltLogger.WithFields(log.Fields{
//        "Error": err,
//        "line " : line,
//      }).Fatal("Can not Convert ..")
//      continue
//    }
//
//    olt.OltStats = append(olt.OltStats, data)
//  }
  for content.Scan(){
    var data openolt.PortStatistics
    line:=content.Text()
    err:= json.Unmarshal([]byte(line), &data)

    if err !=nil {
        oltLogger.WithFields(log.Fields{
        "Error": err,
        "line " : line,
      }).Fatal("Can not Convert ..")
      continue
    }

    olt.OltStats = append(olt.OltStats, data)

  }
  oltLogger.Debug("Complete.. %v", len(olt.OltStats))
}

func (o *OltDevice) InitOlt() {

	if o.OltServer == nil {
		o.OltServer, _ = o.StartOltServer()
	} else {
		oltLogger.Fatal("OLT server already running.")
	}

	// create new channel for processOltMessages Go routine
	o.channel = make(chan types.Message)

	// FIXME we are assuming we have only one NNI
	if o.Nnis[0] != nil {
		// NOTE we want to make sure the state is down when we initialize the OLT,
		// the NNI may be in a bad state after a disable/reboot as we are not disabling it for
		// in-band management
		o.Nnis[0].OperState.SetState("down")
	}

	for ponId := range o.Pons {
		// initialize the resource maps for every PON Ports
		olt.AllocIDs[uint32(ponId)] = make(map[uint32]map[uint32]map[int32]map[uint64]bool)
		olt.GemPortIDs[uint32(ponId)] = make(map[uint32]map[uint32]map[int32]map[uint64]bool)
	}
}

func (o *OltDevice) RestartOLT() error {

	o.PreviouslyConnected = false

	softReboot := false
	rebootDelay := common.Config.Olt.OltRebootDelay

	oltLogger.WithFields(log.Fields{
		"oltId": o.ID,
	}).Infof("Simulating OLT restart... (%ds)", rebootDelay)

	if o.InternalState.Is(OltInternalStateEnabled) {
		oltLogger.WithFields(log.Fields{
			"oltId": o.ID,
		}).Info("This is an OLT soft reboot")
		softReboot = true
	}

	// transition internal state to deleted
	if err := o.InternalState.Event(OltInternalTxDelete); err != nil {
		oltLogger.WithFields(log.Fields{
			"oltId": o.ID,
		}).Errorf("Error deleting OLT: %v", err)
		return err
	}

	if softReboot {
		for _, pon := range o.Pons {
			/* No need to send pon events on olt soft reboot
			if pon.InternalState.Current() == "enabled" {
				// disable PONs
				msg := types.Message{
					Type: types.PonIndication,
					Data: types.PonIndicationMessage{
						OperState: types.DOWN,
						PonPortID: pon.ID,
					},
				}
				o.channel <- msg
			}
			*/
			for _, onu := range pon.Onus {
				err := onu.InternalState.Event(OnuTxDisable)
				oltLogger.WithFields(log.Fields{
					"oltId": o.ID,
					"onuId": onu.ID,
				}).Errorf("Error disabling ONUs on OLT soft reboot: %v", err)
			}
		}
	} else {
		// PONs are already handled in the Disable call
		for _, pon := range olt.Pons {
			// ONUs are not automatically disabled when a PON goes down
			// as it's possible that it's an admin down and in that case the ONUs need to keep their state
			for _, onu := range pon.Onus {
				err := onu.InternalState.Event(OnuTxDisable)
				oltLogger.WithFields(log.Fields{
					"oltId": o.ID,
					"onuId": onu.ID,
					"OnuSn": onu.Sn(),
				}).Errorf("Error disabling ONUs on OLT reboot: %v", err)
			}
		}
	}

	time.Sleep(1 * time.Second) // we need to give the OLT the time to respond to all the pending gRPC request before stopping the server
	o.StopOltServer()

	// terminate the OLT's processOltMessages go routine
	close(o.channel)

	oltLogger.WithFields(log.Fields{
		"oltId": o.ID,
	}).Infof("Waiting OLT restart for... (%ds)", rebootDelay)

	//Prevents Enable to progress before the reboot is completed (VOL-4616)
	o.Lock()
	o.enableContextCancel()
	time.Sleep(time.Duration(rebootDelay) * time.Second)
	o.Unlock()
	o.signature = uint32(time.Now().Unix())

	if err := o.InternalState.Event(OltInternalTxInitialize); err != nil {
		oltLogger.WithFields(log.Fields{
			"oltId": o.ID,
		}).Errorf("Error initializing OLT: %v", err)
		return err
	}
	oltLogger.WithFields(log.Fields{
		"oltId": o.ID,
	}).Info("OLT restart completed")
	return nil
}

// newOltServer launches a new grpc server for OpenOLT
func (o *OltDevice) newOltServer() (*grpc.Server, error) {
	address := common.Config.BBSim.OpenOltAddress
	lis, err := net.Listen("tcp", address)
	if err != nil {
		oltLogger.Fatalf("OLT failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()

	openolt.RegisterOpenoltServer(grpcServer, o)
  bossopenolt.RegisterBossOpenoltServer(grpcServer, o)
	reflection.Register(grpcServer)

	go func() { _ = grpcServer.Serve(lis) }()
	oltLogger.Debugf("OLT listening on %v", address)

	return grpcServer, nil
}

// StartOltServer will create the grpc server that VOLTHA uses
// to communicate with the device
func (o *OltDevice) StartOltServer() (*grpc.Server, error) {
	oltServer, err := o.newOltServer()
	if err != nil {
		oltLogger.WithFields(log.Fields{
			"err": err,
		}).Error("Cannot OLT gRPC server")
		return nil, err
	}
	return oltServer, nil
}

// StopOltServer stops the OpenOLT grpc server
func (o *OltDevice) StopOltServer() {
	if o.OltServer != nil {
		oltLogger.WithFields(log.Fields{
			"oltId": o.SerialNumber,
		}).Warnf("Stopping OLT gRPC server")
		o.OltServer.Stop()
		o.OltServer = nil
	} else {
		oltLogger.WithFields(log.Fields{
			"oltId": o.SerialNumber,
		}).Warnf("OLT gRPC server is already stopped")
	}
}

// Device Methods

// Enable implements the OpenOLT EnableIndicationServer functionality
func (o *OltDevice) Enable(stream openolt.Openolt_EnableIndicationServer) error {
	oltLogger.Debug("Enable OLT called")

	if o.InternalState.Is(OltInternalStateDeleted) {
		err := fmt.Errorf("Cannot enable OLT while it is rebooting")
		oltLogger.WithFields(log.Fields{
			"oltId":         o.SerialNumber,
			"internalState": o.InternalState.Current(),
		}).Error(err)
		return err
	}

	rebootFlag := false

	// If enabled has already been called then an enabled context has
	// been created. If this is the case then we want to cancel all the
	// proessing loops associated with that enable before we recreate
	// new ones
	o.Lock()
	if o.enableContext != nil && o.enableContextCancel != nil {
		oltLogger.Info("This is an OLT reboot or a reconcile")
		o.enableContextCancel()
		rebootFlag = true
		time.Sleep(1 * time.Second)
	}
	o.enableContext, o.enableContextCancel = context.WithCancel(context.TODO())
	o.Unlock()

	wg := sync.WaitGroup{}

	o.OpenoltStream = stream

	// create Go routine to process all OLT events
	wg.Add(1)
	go o.processOltMessages(o.enableContext, stream, &wg)

	// enable the OLT
	oltMsg := types.Message{
		Type: types.OltIndication,
		Data: types.OltIndicationMessage{
			OperState: types.UP,
		},
	}
	o.channel <- oltMsg

	// send NNI Port Indications
	for _, nni := range o.Nnis {
		msg := types.Message{
			Type: types.NniIndication,
			Data: types.NniIndicationMessage{
				OperState: types.UP,
				NniPortID: nni.ID,
			},
		}
		o.channel <- msg
	}

	if rebootFlag {
		for _, pon := range o.Pons {
			if pon.InternalState.Current() == "disabled" {
				msg := types.Message{
					Type: types.PonIndication,
					Data: types.PonIndicationMessage{
						OperState: types.UP,
						PonPortID: pon.ID,
					},
				}
				o.channel <- msg
			}
			// when the enableContext was canceled the ONUs stopped listening on the channel
			for _, onu := range pon.Onus {
				if o.ControlledActivation != OnlyONU {
					onu.ReDiscoverOnu(true)
				}
				go onu.ProcessOnuMessages(o.enableContext, stream, nil)

				// update the stream on all the services
				for _, uni := range onu.UniPorts {
					uni.UpdateStream(stream)
				}
			}
		}
	} else {

		// 1. controlledActivation == Default: Send both PON and ONUs indications
		// 2. controlledActivation == only-onu: that means only ONUs will be controlled activated, so auto send PON indications

		if o.ControlledActivation == Default || o.ControlledActivation == OnlyONU {
			// send PON Port indications
			for _, pon := range o.Pons {
				msg := types.Message{
					Type: types.PonIndication,
					Data: types.PonIndicationMessage{
						OperState: types.UP,
						PonPortID: pon.ID,
					},
				}
				o.channel <- msg
			}
		}
	}

	if !o.enablePerf {
		// Start a go routine to send periodic port stats to openolt adapter
		wg.Add(1)
		go o.periodicPortStats(o.enableContext, &wg, stream)
	}

	wg.Wait()
	oltLogger.WithFields(log.Fields{
		"stream": stream,
	}).Debug("OpenOLT Stream closed")

	return nil
}

func (o *OltDevice) periodicPortStats(ctx context.Context, wg *sync.WaitGroup, stream openolt.Openolt_EnableIndicationServer) {
	//var portStats *openolt.PortStatistics

  count := 0
loop:
	for {
		select {
		case <-time.After(time.Duration(o.PortStatsInterval) * time.Second):
			// send NNI port stats
//			for _, port := range o.Nnis {
//				incrementStat := true
//				if port.OperState.Current() == "down" {
//					incrementStat = false
//				}
//				portStats, port.PacketCount = getPortStats(port.PacketCount, incrementStat)
//				o.sendPortStatsIndication(portStats, port.ID, port.Type, stream)
//			}
//
//			// send PON port stats
//			for _, port := range o.Pons {
//				incrementStat := true
//				// do not increment port stats if PON port is down or no ONU is activated on PON port
//				if port.OperState.Current() == "down" || port.GetNumOfActiveOnus() < 1 {
//					incrementStat = false
//				}
//				portStats, port.PacketCount = getPortStats(port.PacketCount, incrementStat)
//				o.sendPortStatsIndication(portStats, port.ID, port.Type, stream)
//			}
      sendStat := o.OltStats[count]
      o.send25GPortStatsIndication(&sendStat, stream)
      count++
      if len(o.OltStats)==count{
        count =0
      }
		case <-ctx.Done():
			oltLogger.Debug("Stop sending port stats")
			break loop
		}
	}
	wg.Done()
}

// Helpers method

func (o *OltDevice) SetAlarm(interfaceId uint32, interfaceType string, alarmStatus string) error {

	switch interfaceType {
	case "nni":
		if !o.HasNni(interfaceId) {
			return status.Errorf(codes.NotFound, strconv.Itoa(int(interfaceId))+" NNI not present in olt")
		}

	case "pon":
		if !o.HasPon(interfaceId) {
			return status.Errorf(codes.NotFound, strconv.Itoa(int(interfaceId))+" PON not present in olt")
		}
	}

	alarmIndication := &openolt.AlarmIndication{
		Data: &openolt.AlarmIndication_LosInd{LosInd: &openolt.LosIndication{
			Status: alarmStatus,
			IntfId: InterfaceIDToPortNo(interfaceId, interfaceType),
		}},
	}

	msg := types.Message{
		Type: types.AlarmIndication,
		Data: alarmIndication,
	}

	o.channel <- msg

	return nil
}

func (o *OltDevice) HasNni(id uint32) bool {
	for _, intf := range o.Nnis {
		if intf.ID == id {
			return true
		}
	}
	return false
}

func (o *OltDevice) HasPon(id uint32) bool {
	for _, intf := range o.Pons {
		if intf.ID == id {
			return true
		}
	}
	return false
}

func (o *OltDevice) GetPonById(id uint32) (*PonPort, error) {
	for _, pon := range o.Pons {
		if pon.ID == id {
			return pon, nil
		}
	}
	return nil, fmt.Errorf("Cannot find PonPort with id %d in OLT %d", id, o.ID)
}

func (o *OltDevice) getNniById(id uint32) (*NniPort, error) {
	for _, nni := range o.Nnis {
		if nni.ID == id {
			return nni, nil
		}
	}
	return nil, fmt.Errorf("Cannot find NniPort with id %d in OLT %d", id, o.ID)
}

func (o *OltDevice) sendAlarmIndication(alarmInd *openolt.AlarmIndication, stream openolt.Openolt_EnableIndicationServer) {
	data := &openolt.Indication_AlarmInd{AlarmInd: alarmInd}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		oltLogger.Errorf("Failed to send Alarm Indication: %v", err)
		return
	}

	oltLogger.WithFields(log.Fields{
		"AlarmIndication": alarmInd,
	}).Debug("Sent Indication_AlarmInd")
}

func (o *OltDevice) sendOltIndication(msg types.OltIndicationMessage, stream openolt.Openolt_EnableIndicationServer) {
	data := &openolt.Indication_OltInd{OltInd: &openolt.OltIndication{OperState: msg.OperState.String()}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		oltLogger.Errorf("Failed to send Indication_OltInd: %v", err)
		return
	}

	oltLogger.WithFields(log.Fields{
		"OperState": msg.OperState,
	}).Debug("Sent Indication_OltInd")
}

func (o *OltDevice) sendNniIndication(msg types.NniIndicationMessage, stream openolt.Openolt_EnableIndicationServer) {
	nni, _ := o.getNniById(msg.NniPortID)
	if msg.OperState == types.UP {
		if err := nni.OperState.Event("enable"); err != nil {
			log.WithFields(log.Fields{
				"Type":      nni.Type,
				"IntfId":    nni.ID,
				"OperState": nni.OperState.Current(),
			}).Errorf("Can't move NNI Port to enabled state: %v", err)
		}
	} else if msg.OperState == types.DOWN {
		if err := nni.OperState.Event("disable"); err != nil {
			log.WithFields(log.Fields{
				"Type":      nni.Type,
				"IntfId":    nni.ID,
				"OperState": nni.OperState.Current(),
			}).Errorf("Can't move NNI Port to disable state: %v", err)
		}
	}
	// NOTE Operstate may need to be an integer
	operData := &openolt.Indication_IntfOperInd{IntfOperInd: &openolt.IntfOperIndication{
		Type:      nni.Type,
		IntfId:    nni.ID,
		OperState: nni.OperState.Current(),
		Speed:     o.NniSpeed,
	}}

	if err := stream.Send(&openolt.Indication{Data: operData}); err != nil {
		oltLogger.Errorf("Failed to send Indication_IntfOperInd for NNI: %v", err)
		return
	}

	oltLogger.WithFields(log.Fields{
		"Type":      nni.Type,
		"IntfId":    nni.ID,
		"OperState": nni.OperState.Current(),
		"Speed":     o.NniSpeed,
	}).Debug("Sent Indication_IntfOperInd for NNI")
}

func (o *OltDevice) sendPonIndication(ponPortID uint32) {

	stream := o.OpenoltStream
	pon, _ := o.GetPonById(ponPortID)
	// Send IntfIndication for PON port
	discoverData := &openolt.Indication_IntfInd{IntfInd: &openolt.IntfIndication{
		IntfId:    pon.ID,
		OperState: pon.OperState.Current(),
	}}

	if err := stream.Send(&openolt.Indication{Data: discoverData}); err != nil {
		oltLogger.Errorf("Failed to send Indication_IntfInd: %v", err)
		return
	}

	oltLogger.WithFields(log.Fields{
		"IntfId":    pon.ID,
		"OperState": pon.OperState.Current(),
	}).Debug("Sent Indication_IntfInd for PON")

	// Send IntfOperIndication for PON port
	operData := &openolt.Indication_IntfOperInd{IntfOperInd: &openolt.IntfOperIndication{
		Type:      pon.Type,
		IntfId:    pon.ID,
		OperState: pon.OperState.Current(),
	}}

	if err := stream.Send(&openolt.Indication{Data: operData}); err != nil {
		oltLogger.Errorf("Failed to send Indication_IntfOperInd for PON: %v", err)
		return
	}

	oltLogger.WithFields(log.Fields{
		"Type":      pon.Type,
		"IntfId":    pon.ID,
		"OperState": pon.OperState.Current(),
	}).Debug("Sent Indication_IntfOperInd for PON")
}

func (o *OltDevice) sendPortStatsIndication(stats *openolt.PortStatistics, portID uint32, portType string, stream openolt.Openolt_EnableIndicationServer) {
	if o.InternalState.Current() == OltInternalStateEnabled {
		oltLogger.WithFields(log.Fields{
			"Type":   portType,
			"IntfId": portID,
		}).Debug("Sending port stats")
		stats.IntfId = InterfaceIDToPortNo(portID, portType)
		data := &openolt.Indication_PortStats{
			PortStats: stats,
		}

		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			oltLogger.Errorf("Failed to send PortStats: %v", err)
			return
		}
	}
}
func (o *OltDevice) send25GPortStatsIndication(stats *openolt.PortStatistics,stream openolt.Openolt_EnableIndicationServer) {
	if o.InternalState.Current() == OltInternalStateEnabled {
		oltLogger.WithFields(log.Fields{
			"Stats": stats,
		}).Debug("Sending port stats")
//		stats.IntfId = InterfaceIDToPortNo(portID, portType)
    if !latencyFlag {
      stats.BipErrors = 0
		  oltLogger.WithFields(log.Fields{
			  "Stats": stats,
		  }).Debug("latency not yet")
    }
		data := &openolt.Indication_PortStats{
			PortStats: stats,
		}
		  oltLogger.WithFields(log.Fields{
			  "Stats": data,
		  }).Debug("Send data")

		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			oltLogger.Errorf("Failed to send PortStats: %v", err)
			return
		}
	}
}


// processOltMessages handles messages received over the OpenOLT interface
func (o *OltDevice) processOltMessages(ctx context.Context, stream types.Stream, wg *sync.WaitGroup) {
	oltLogger.WithFields(log.Fields{
		"stream": stream,
	}).Debug("Starting OLT Indication Channel")
	ch := o.channel

loop:
	for {
		select {
		case <-ctx.Done():
			oltLogger.Debug("OLT Indication processing canceled via context")
			break loop
		// do not terminate this loop if the stream is closed,
		// when we restart the gRPC server it will automatically reconnect and we need this loop to send indications
		//case <-stream.Context().Done():
		//	oltLogger.Debug("OLT Indication processing canceled via stream context")
		//	break loop
		case message, ok := <-ch:
			if !ok {
				if ctx.Err() != nil {
					oltLogger.WithField("err", ctx.Err()).Error("OLT EnableContext error")
				}
				oltLogger.Warn("OLT Indication processing canceled via closed channel")
				break loop
			}

			oltLogger.WithFields(log.Fields{
				"oltId":       o.ID,
				"messageType": message.Type,
			}).Debug("Received message")

			switch message.Type {
			case types.OltIndication:
				msg, _ := message.Data.(types.OltIndicationMessage)
				if msg.OperState == types.UP {
					_ = o.InternalState.Event(OltInternalTxEnable)
					_ = o.OperState.Event("enable")
				} else if msg.OperState == types.DOWN {
					_ = o.InternalState.Event(OltInternalTxDisable)
					_ = o.OperState.Event("disable")
				}
				o.sendOltIndication(msg, stream)
			case types.AlarmIndication:
				alarmInd, _ := message.Data.(*openolt.AlarmIndication)
				o.sendAlarmIndication(alarmInd, stream)
			case types.NniIndication:
				msg, _ := message.Data.(types.NniIndicationMessage)
				o.sendNniIndication(msg, stream)
			case types.PonIndication:
				msg, _ := message.Data.(types.PonIndicationMessage)
				pon, _ := o.GetPonById(msg.PonPortID)
				if msg.OperState == types.UP {
					if err := pon.OperState.Event("enable"); err != nil {
						oltLogger.WithFields(log.Fields{
							"IntfId": msg.PonPortID,
							"Err":    err,
						}).Error("Can't Enable Oper state for PON Port")
					}
					if err := pon.InternalState.Event("enable"); err != nil {
						oltLogger.WithFields(log.Fields{
							"IntfId": msg.PonPortID,
							"Err":    err,
						}).Error("Can't Enable Internal state for PON Port")
					}
				} else if msg.OperState == types.DOWN {
					if err := pon.OperState.Event("disable"); err != nil {
						oltLogger.WithFields(log.Fields{
							"IntfId": msg.PonPortID,
							"Err":    err,
						}).Error("Can't Disable Oper state for PON Port")
					}
					if err := pon.InternalState.Event("disable"); err != nil {
						oltLogger.WithFields(log.Fields{
							"IntfId": msg.PonPortID,
							"Err":    err,
						}).Error("Can't Disable Internal state for PON Port")
					}
				}
			default:
				oltLogger.Warnf("Received unknown message data %v for type %v in OLT Channel", message.Data, message.Type)
			}
		}
	}
	wg.Done()
	oltLogger.WithFields(log.Fields{
		"stream": stream,
	}).Warn("Stopped handling OLT Indication Channel")
}

// returns an ONU with a given Serial Number
func (o *OltDevice) FindOnuBySn(serialNumber string) (*Onu, error) {
	// NOTE this function can be a performance bottleneck when we have many ONUs,
	// memoizing it will remove the bottleneck
	for _, pon := range o.Pons {
		for _, onu := range pon.Onus {
			if onu.Sn() == serialNumber {
				return onu, nil
			}
		}
	}

	return &Onu{}, fmt.Errorf("cannot-find-onu-by-serial-number-%s", serialNumber)
}

// returns an ONU with a given interface/Onu Id
func (o *OltDevice) FindOnuById(intfId uint32, onuId uint32) (*Onu, error) {
	// NOTE this function can be a performance bottleneck when we have many ONUs,
	// memoizing it will remove the bottleneck
	for _, pon := range o.Pons {
		if pon.ID == intfId {
			for _, onu := range pon.Onus {
				if onu.ID == onuId {
					return onu, nil
				}
			}
		}
	}
	return &Onu{}, fmt.Errorf("cannot-find-onu-by-id-%v-%v", intfId, onuId)
}

// returns a Service with a given Mac Address
func (o *OltDevice) FindServiceByMacAddress(mac net.HardwareAddr) (ServiceIf, error) {
	// NOTE this function can be a performance bottleneck when we have many ONUs,
	// memoizing it will remove the bottleneck
	for _, pon := range o.Pons {
		for _, onu := range pon.Onus {
			s, err := onu.findServiceByMacAddress(mac)
			if err == nil {
				return s, nil
			}
		}
	}

	return nil, fmt.Errorf("cannot-find-service-by-mac-address-%s", mac)
}

// GRPC Endpoints

func (o *OltDevice) ActivateOnu(context context.Context, onu *openolt.Onu) (*openolt.Empty, error) {

	pon, _ := o.GetPonById(onu.IntfId)

	// Enable the resource maps for this ONU
	olt.AllocIDs[onu.IntfId][onu.OnuId] = make(map[uint32]map[int32]map[uint64]bool)
	olt.GemPortIDs[onu.IntfId][onu.OnuId] = make(map[uint32]map[int32]map[uint64]bool)

	_onu, _ := pon.GetOnuBySn(onu.SerialNumber)

	publishEvent("ONU-activate-indication-received", int32(onu.IntfId), int32(onu.OnuId), _onu.Sn())
	oltLogger.WithFields(log.Fields{
		"OnuSn": _onu.Sn(),
	}).Info("Received ActivateOnu call from VOLTHA")

	_onu.SetID(onu.OnuId)

	if err := _onu.InternalState.Event(OnuTxEnable); err != nil {
		oltLogger.WithFields(log.Fields{
			"IntfId": _onu.PonPortID,
			"OnuSn":  _onu.Sn(),
			"OnuId":  _onu.ID,
		}).Infof("Failed to transition ONU to %s state: %s", OnuStateEnabled, err.Error())
	}

	// NOTE we need to immediately activate the ONU or the OMCI state machine won't start

	return new(openolt.Empty), nil
}

func (o *OltDevice) DeactivateOnu(_ context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	oltLogger.Error("DeactivateOnu not implemented")
	return new(openolt.Empty), nil
}

func (o *OltDevice) DeleteOnu(_ context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"IntfId": onu.IntfId,
		"OnuId":  onu.OnuId,
	}).Info("Received DeleteOnu call from VOLTHA")

	pon, err := o.GetPonById(onu.IntfId)
	if err != nil {
		oltLogger.WithFields(log.Fields{
			"OnuId":  onu.OnuId,
			"IntfId": onu.IntfId,
			"err":    err,
		}).Error("Can't find PonPort")
	}
	_onu, err := pon.GetOnuById(onu.OnuId)
	if err != nil {
		oltLogger.WithFields(log.Fields{
			"OnuId":  onu.OnuId,
			"IntfId": onu.IntfId,
			"err":    err,
		}).Error("Can't find Onu")
	}

	if _onu.InternalState.Current() != OnuStateDisabled {
		if err := _onu.InternalState.Event(OnuTxDisable); err != nil {
			oltLogger.WithFields(log.Fields{
				"IntfId": _onu.PonPortID,
				"OnuSn":  _onu.Sn(),
				"OnuId":  _onu.ID,
			}).Infof("Failed to transition ONU to %s state: %s", OnuStateDisabled, err.Error())
		}
	}

	// ONU Re-Discovery
	if o.InternalState.Current() == OltInternalStateEnabled && pon.InternalState.Current() == "enabled" {
		go _onu.ReDiscoverOnu(false)
	}

	return new(openolt.Empty), nil
}

func (o *OltDevice) DisableOlt(context.Context, *openolt.Empty) (*openolt.Empty, error) {
	// NOTE when we disable the OLT should we disable NNI, PONs and ONUs altogether?
	oltLogger.WithFields(log.Fields{
		"oltId": o.ID,
	}).Info("Disabling OLT")
	publishEvent("OLT-disable-received", -1, -1, "")

	for _, pon := range o.Pons {
		if pon.InternalState.Current() == "enabled" {
			// disable PONs
			msg := types.Message{
				Type: types.PonIndication,
				Data: types.PonIndicationMessage{
					OperState: types.DOWN,
					PonPortID: pon.ID,
				},
			}
			o.channel <- msg
		}
	}

	// Note that we are not disabling the NNI as the real OLT does not.
	// The reason for that is in-band management

	// disable OLT
	oltMsg := types.Message{
		Type: types.OltIndication,
		Data: types.OltIndicationMessage{
			OperState: types.DOWN,
		},
	}
	o.channel <- oltMsg

	return new(openolt.Empty), nil
}

func (o *OltDevice) DisablePonIf(_ context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	oltLogger.Infof("DisablePonIf request received for PON %d", intf.IntfId)
	ponID := intf.GetIntfId()
	pon, _ := o.GetPonById(intf.IntfId)

	msg := types.Message{
		Type: types.PonIndication,
		Data: types.PonIndicationMessage{
			OperState: types.DOWN,
			PonPortID: ponID,
		},
	}
	o.channel <- msg

	for _, onu := range pon.Onus {

		onuIndication := types.OnuIndicationMessage{
			OperState: types.DOWN,
			PonPortID: ponID,
			OnuID:     onu.ID,
			OnuSN:     onu.SerialNumber,
		}
		onu.sendOnuIndication(onuIndication, o.OpenoltStream)

	}

	return new(openolt.Empty), nil
}

func (o *OltDevice) EnableIndication(_ *openolt.Empty, stream openolt.Openolt_EnableIndicationServer) error {
	oltLogger.WithField("oltId", o.ID).Info("OLT receives EnableIndication call from VOLTHA")
	publishEvent("OLT-enable-received", -1, -1, "")
	return o.Enable(stream)
}

func (o *OltDevice) EnablePonIf(_ context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	oltLogger.Infof("EnablePonIf request received for PON %d", intf.IntfId)
	ponID := intf.GetIntfId()
	pon, _ := o.GetPonById(intf.IntfId)

	msg := types.Message{
		Type: types.PonIndication,
		Data: types.PonIndicationMessage{
			OperState: types.UP,
			PonPortID: ponID,
		},
	}
	o.channel <- msg

	for _, onu := range pon.Onus {

		onuIndication := types.OnuIndicationMessage{
			OperState: types.UP,
			PonPortID: ponID,
			OnuID:     onu.ID,
			OnuSN:     onu.SerialNumber,
		}
		onu.sendOnuIndication(onuIndication, o.OpenoltStream)

	}

	return new(openolt.Empty), nil
}

func (o *OltDevice) FlowAdd(ctx context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"IntfId":    flow.AccessIntfId,
		"OnuId":     flow.OnuId,
		"EthType":   fmt.Sprintf("%x", flow.Classifier.EthType),
		"InnerVlan": flow.Classifier.IVid,
		"OuterVlan": flow.Classifier.OVid,
		"FlowType":  flow.FlowType,
		"FlowId":    flow.FlowId,
		"UniID":     flow.UniId,
		"PortNo":    flow.PortNo,
	}).Debugf("OLT receives FlowAdd")

	flowKey := FlowKey{}
	if !o.enablePerf {
		flowKey = FlowKey{ID: flow.FlowId}
		olt.Flows.Store(flowKey, *flow)
	}

	if flow.AccessIntfId == -1 {
		oltLogger.WithFields(log.Fields{
			"FlowId": flow.FlowId,
		}).Debug("Adding OLT flow")
	} else if flow.FlowType == "multicast" {
		oltLogger.WithFields(log.Fields{
			"Cookie":           flow.Cookie,
			"DstPort":          flow.Classifier.DstPort,
			"EthType":          fmt.Sprintf("%x", flow.Classifier.EthType),
			"FlowId":           flow.FlowId,
			"FlowType":         flow.FlowType,
			"GemportId":        flow.GemportId,
			"InnerVlan":        flow.Classifier.IVid,
			"IntfId":           flow.AccessIntfId,
			"IpProto":          flow.Classifier.IpProto,
			"OnuId":            flow.OnuId,
			"OuterVlan":        flow.Classifier.OVid,
			"PortNo":           flow.PortNo,
			"SrcPort":          flow.Classifier.SrcPort,
			"UniID":            flow.UniId,
			"ClassifierOPbits": flow.Classifier.OPbits,
		}).Debug("Adding OLT multicast flow")
	} else {
		pon, err := o.GetPonById(uint32(flow.AccessIntfId))
		if err != nil {
			oltLogger.WithFields(log.Fields{
				"OnuId":  flow.OnuId,
				"IntfId": flow.AccessIntfId,
				"err":    err,
			}).Error("Can't find PonPort")
		}
		onu, err := pon.GetOnuById(uint32(flow.OnuId))
		if err != nil {
			oltLogger.WithFields(log.Fields{
				"OnuId":  flow.OnuId,
				"IntfId": flow.AccessIntfId,
				"err":    err,
			}).Error("Can't find Onu")
			return nil, err
		}

		// if the ONU is disabled reject the flow
		// as per VOL-4061 there is a small window during which the ONU is disabled
		// but the port has not been reported as down to ONOS
		if onu.InternalState.Is(OnuStatePonDisabled) || onu.InternalState.Is(OnuStateDisabled) {
			oltLogger.WithFields(log.Fields{
				"OnuId":         flow.OnuId,
				"IntfId":        flow.AccessIntfId,
				"Flow":          flow,
				"SerialNumber":  onu.Sn(),
				"InternalState": onu.InternalState.Current(),
			}).Error("rejected-flow-because-of-onu-state")
			return nil, fmt.Errorf("onu-%s-is-currently-%s", onu.Sn(), onu.InternalState.Current())
		}

		if !o.enablePerf {
			onu.Flows = append(onu.Flows, flowKey)
			// Generate event on first flow for ONU
			if len(onu.Flows) == 1 {
				publishEvent("Flow-add-received", int32(onu.PonPortID), int32(onu.ID), onu.Sn())
			}
		}

		// validate that the flow reference correct IDs (Alloc, Gem)
		if err := o.validateFlow(flow); err != nil {
			oltLogger.WithFields(log.Fields{
				"OnuId":        flow.OnuId,
				"IntfId":       flow.AccessIntfId,
				"Flow":         flow,
				"SerialNumber": onu.Sn(),
				"err":          err,
			}).Error("invalid-flow-for-onu")
			return nil, err
		}

		o.storeGemPortIdByFlow(flow)
		o.storeAllocId(flow)

		msg := types.Message{
			Type: types.FlowAdd,
			Data: types.OnuFlowUpdateMessage{
				PonPortID: pon.ID,
				OnuID:     onu.ID,
				Flow:      flow,
			},
		}
		onu.Channel <- msg
	}

	return new(openolt.Empty), nil
}

// FlowRemove request from VOLTHA
func (o *OltDevice) FlowRemove(_ context.Context, flow *openolt.Flow) (*openolt.Empty, error) {

	oltLogger.WithFields(log.Fields{
		"AllocId":       flow.AllocId,
		"Cookie":        flow.Cookie,
		"FlowId":        flow.FlowId,
		"FlowType":      flow.FlowType,
		"GemportId":     flow.GemportId,
		"IntfId":        flow.AccessIntfId,
		"OnuId":         flow.OnuId,
		"PortNo":        flow.PortNo,
		"UniID":         flow.UniId,
		"ReplicateFlow": flow.ReplicateFlow,
		"PbitToGemport": flow.PbitToGemport,
	}).Debug("OLT receives FlowRemove")

	olt.freeGemPortId(flow)
	olt.freeAllocId(flow)

	if !o.enablePerf { // remove only if flow were stored
		flowKey := FlowKey{ID: flow.FlowId}
		// Check if flow exists
		storedFlowIntf, ok := o.Flows.Load(flowKey)
		if !ok {
			oltLogger.Errorf("Flow %v not found", flow)
			return new(openolt.Empty), status.Errorf(codes.NotFound, "Flow not found")
		}

		storedFlow := storedFlowIntf.(openolt.Flow)

		// if its ONU flow remove it from ONU also
		if storedFlow.AccessIntfId != -1 {
			pon, err := o.GetPonById(uint32(storedFlow.AccessIntfId))
			if err != nil {
				oltLogger.WithFields(log.Fields{
					"OnuId":  storedFlow.OnuId,
					"IntfId": storedFlow.AccessIntfId,
					"PONs":   olt.Pons,
					"err":    err,
				}).Error("PON-port-not-found")
				return new(openolt.Empty), nil
			}
			onu, err := pon.GetOnuById(uint32(storedFlow.OnuId))
			if err != nil {
				oltLogger.WithFields(log.Fields{
					"OnuId":  storedFlow.OnuId,
					"IntfId": storedFlow.AccessIntfId,
					"err":    err,
				}).Error("ONU-not-found")
				return new(openolt.Empty), nil
			}
			onu.DeleteFlow(flowKey)
			publishEvent("Flow-remove-received", int32(onu.PonPortID), int32(onu.ID), onu.Sn())
		}

		// delete from olt flows
		o.Flows.Delete(flowKey)
	}

	if flow.AccessIntfId == -1 {
		oltLogger.WithFields(log.Fields{
			"FlowId": flow.FlowId,
		}).Debug("Removing OLT flow")
	} else if flow.FlowType == "multicast" {
		oltLogger.WithFields(log.Fields{
			"FlowId": flow.FlowId,
		}).Debug("Removing OLT multicast flow")
	} else {

		onu, err := o.GetOnuByFlowId(flow.FlowId)
		if err != nil {
			oltLogger.WithFields(log.Fields{
				"OnuId":  flow.OnuId,
				"IntfId": flow.AccessIntfId,
				"err":    err,
			}).Error("Can't find Onu")
			return nil, err
		}

		msg := types.Message{
			Type: types.FlowRemoved,
			Data: types.OnuFlowUpdateMessage{
				Flow: flow,
			},
		}
		onu.Channel <- msg
	}

	return new(openolt.Empty), nil
}

func (o *OltDevice) HeartbeatCheck(context.Context, *openolt.Empty) (*openolt.Heartbeat, error) {
	res := openolt.Heartbeat{HeartbeatSignature: o.signature}
	oltLogger.WithFields(log.Fields{
		"signature": res.HeartbeatSignature,
	}).Debug("HeartbeatCheck")
	return &res, nil
}

func (o *OltDevice) GetOnuByFlowId(flowId uint64) (*Onu, error) {
	for _, pon := range o.Pons {
		for _, onu := range pon.Onus {
			for _, fId := range onu.FlowIds {
				if fId == flowId {
					return onu, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("Cannot find Onu by flowId %d", flowId)
}

func (o *OltDevice) GetDeviceInfo(context.Context, *openolt.Empty) (*openolt.DeviceInfo, error) {
	devinfo := &openolt.DeviceInfo{
		Vendor:              common.Config.Olt.Vendor,
		Model:               common.Config.Olt.Model,
		HardwareVersion:     common.Config.Olt.HardwareVersion,
		FirmwareVersion:     common.Config.Olt.FirmwareVersion,
		PonPorts:            uint32(o.NumPon),
		DeviceSerialNumber:  o.SerialNumber,
		DeviceId:            common.Config.Olt.DeviceId,
		PreviouslyConnected: o.PreviouslyConnected,
		Ranges:              []*openolt.DeviceInfo_DeviceResourceRanges{},
	}

	for _, resRange := range common.PonsConfig.Ranges {
		intfIDs := []uint32{}
		for i := resRange.PonRange.StartId; i <= resRange.PonRange.EndId; i++ {
			intfIDs = append(intfIDs, uint32(i))
		}

		devinfo.Ranges = append(devinfo.Ranges, &openolt.DeviceInfo_DeviceResourceRanges{
			IntfIds:    intfIDs,
			Technology: "ETRI-PON",
			Pools: []*openolt.DeviceInfo_DeviceResourceRanges_Pool{
				{
					Type:    openolt.DeviceInfo_DeviceResourceRanges_Pool_ONU_ID,
					Sharing: openolt.DeviceInfo_DeviceResourceRanges_Pool_DEDICATED_PER_INTF,
					Start:   resRange.OnuRange.StartId,
					End:     resRange.OnuRange.EndId,
				},
				{
					Type:    openolt.DeviceInfo_DeviceResourceRanges_Pool_ALLOC_ID,
					Sharing: openolt.DeviceInfo_DeviceResourceRanges_Pool_DEDICATED_PER_INTF,
					Start:   resRange.AllocIdRange.StartId,
					End:     resRange.AllocIdRange.EndId,
				},
				{
					Type:    openolt.DeviceInfo_DeviceResourceRanges_Pool_GEMPORT_ID,
					Sharing: openolt.DeviceInfo_DeviceResourceRanges_Pool_DEDICATED_PER_INTF,
					Start:   resRange.GemportRange.StartId,
					End:     resRange.GemportRange.EndId,
				},
			},
		})
	}

	oltLogger.WithFields(log.Fields{
		"Vendor":              devinfo.Vendor,
		"Model":               devinfo.Model,
		"HardwareVersion":     devinfo.HardwareVersion,
		"FirmwareVersion":     devinfo.FirmwareVersion,
		"PonPorts":            devinfo.PonPorts,
		"DeviceSerialNumber":  devinfo.DeviceSerialNumber,
		"DeviceId":            devinfo.DeviceId,
		"PreviouslyConnected": devinfo.PreviouslyConnected,
	}).Info("OLT receives GetDeviceInfo call from VOLTHA")
  oltLogger.WithFields(log.Fields{
    "devInfo" : devinfo,
  }).Debug("GetDeviceInfo")
	// once we connect, set the flag
	o.PreviouslyConnected = true

	return devinfo, nil
}

func (o *OltDevice) OmciMsgOut(ctx context.Context, omci_msg *openolt.OmciMsg) (*openolt.Empty, error) {
	pon, err := o.GetPonById(omci_msg.IntfId)
	if err != nil {
		oltLogger.WithFields(log.Fields{
			"error":  err,
			"onu_id": omci_msg.OnuId,
			"pon_id": omci_msg.IntfId,
		}).Error("pon ID not found")
		return nil, err
	}

	onu, err := pon.GetOnuById(omci_msg.OnuId)
	if err != nil {
		oltLogger.WithFields(log.Fields{
			"error":  err,
			"onu_id": omci_msg.OnuId,
			"pon_id": omci_msg.IntfId,
		}).Error("onu ID not found")
		return nil, err
	}

	oltLogger.WithFields(log.Fields{
		"IntfId": onu.PonPortID,
		"OnuId":  onu.ID,
		"OnuSn":  onu.Sn(),
	}).Debugf("Received OmciMsgOut")
	omciPkt, omciMsg, err := omcilib.ParseOpenOltOmciPacket(omci_msg.Pkt)
	if err != nil {
		log.WithFields(log.Fields{
			"IntfId":       onu.PonPortID,
			"SerialNumber": onu.Sn(),
			"omciPacket":   hex.EncodeToString(omci_msg.Pkt),
			"err":          err.Error(),
		}).Error("cannot-parse-OMCI-packet")
		return nil, fmt.Errorf("olt-received-malformed-omci-packet")
	}
	if onu.InternalState.Current() == OnuStateDisabled {
		// if the ONU is disabled just drop the message
		log.WithFields(log.Fields{
			"IntfId":       onu.PonPortID,
			"SerialNumber": onu.Sn(),
			"omciBytes":    hex.EncodeToString(omciPkt.Data()),
			"omciPkt":      omciPkt,
			"omciMsgType":  omciMsg.MessageType,
		}).Warn("dropping-omci-message")
	} else {
		msg := types.Message{
			Type: types.OMCI,
			Data: types.OmciMessage{
				OnuSN:   onu.SerialNumber,
				OnuID:   onu.ID,
				OmciMsg: omciMsg,
				OmciPkt: omciPkt,
			},
		}
		onu.Channel <- msg
	}
	return new(openolt.Empty), nil
}

// this gRPC methods receives packets from VOLTHA and sends them to the subscriber on the ONU
func (o *OltDevice) OnuPacketOut(ctx context.Context, onuPkt *openolt.OnuPacket) (*openolt.Empty, error) {
	pon, err := o.GetPonById(onuPkt.IntfId)
	if err != nil {
		oltLogger.WithFields(log.Fields{
			"OnuId":  onuPkt.OnuId,
			"IntfId": onuPkt.IntfId,
			"err":    err,
		}).Error("Can't find PonPort")
	}
	onu, err := pon.GetOnuById(onuPkt.OnuId)
	if err != nil {
		oltLogger.WithFields(log.Fields{
			"OnuId":  onuPkt.OnuId,
			"IntfId": onuPkt.IntfId,
			"err":    err,
		}).Error("Can't find Onu")
	}

	oltLogger.WithFields(log.Fields{
		"IntfId": onu.PonPortID,
		"OnuId":  onu.ID,
		"OnuSn":  onu.Sn(),
		"Packet": hex.EncodeToString(onuPkt.Pkt),
	}).Debug("Received OnuPacketOut")

	rawpkt := gopacket.NewPacket(onuPkt.Pkt, layers.LayerTypeEthernet, gopacket.Default)

	pktType, err := packetHandlers.GetPktType(rawpkt)
	if err != nil {
		onuLogger.WithFields(log.Fields{
			"IntfId": onu.PonPortID,
			"OnuId":  onu.ID,
			"OnuSn":  onu.Sn(),
			"Pkt":    hex.EncodeToString(rawpkt.Data()),
		}).Error("Can't find pktType in packet, droppint it")
		return new(openolt.Empty), nil
	}

	pktMac, err := packetHandlers.GetDstMacAddressFromPacket(rawpkt)
	if err != nil {
		onuLogger.WithFields(log.Fields{
			"IntfId": onu.PonPortID,
			"OnuId":  onu.ID,
			"OnuSn":  onu.Sn(),
			"Pkt":    rawpkt.Data(),
		}).Error("Can't find Dst MacAddress in packet, droppint it")
		return new(openolt.Empty), nil
	}

	msg := types.Message{
		Type: types.OnuPacketOut,
		Data: types.OnuPacketMessage{
			IntfId:     onuPkt.IntfId,
			OnuId:      onuPkt.OnuId,
			PortNo:     onuPkt.PortNo,
			Packet:     rawpkt,
			Type:       pktType,
			MacAddress: pktMac,
		},
	}

	onu.Channel <- msg

	return new(openolt.Empty), nil
}

func (o *OltDevice) Reboot(context.Context, *openolt.Empty) (*openolt.Empty, error) {

	// OLT Reboot is called in two cases:
	// - when an OLT is being removed (voltctl device disable -> voltctl device delete are called, then a new voltctl device create -> voltctl device enable will be issued)
	// - when an OLT needs to be rebooted (voltcl device reboot)

	oltLogger.WithFields(log.Fields{
		"oltId": o.ID,
	}).Info("Shutting down")
	publishEvent("OLT-reboot-received", -1, -1, "")
	go func() { _ = o.RestartOLT() }()
	return new(openolt.Empty), nil
}

func (o *OltDevice) ReenableOlt(context.Context, *openolt.Empty) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"oltId": o.ID,
	}).Info("Received ReenableOlt request from VOLTHA")
	publishEvent("OLT-reenable-received", -1, -1, "")

	// enable OLT
	oltMsg := types.Message{
		Type: types.OltIndication,
		Data: types.OltIndicationMessage{
			OperState: types.UP,
		},
	}
	o.channel <- oltMsg

	for _, pon := range o.Pons {
		if pon.InternalState.Current() == "disabled" {
			msg := types.Message{
				Type: types.PonIndication,
				Data: types.PonIndicationMessage{
					OperState: types.UP,
					PonPortID: pon.ID,
				},
			}
			o.channel <- msg
		}
	}

	return new(openolt.Empty), nil
}

func (o *OltDevice) UplinkPacketOut(context context.Context, packet *openolt.UplinkPacket) (*openolt.Empty, error) {
	pkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)

	err := o.Nnis[0].handleNniPacket(pkt) // FIXME we are assuming we have only one NNI

	if err != nil {
		return nil, err
	}
	return new(openolt.Empty), nil
}

func (o *OltDevice) CollectStatistics(context.Context, *openolt.Empty) (*openolt.Empty, error) {
	oltLogger.Error("CollectStatistics not implemented")
	return new(openolt.Empty), nil
}

//func (o *OltDevice) GetOnuInfo(context context.Context, packet *openolt.Onu) (*openolt.OnuIndication, error) {
//	oltLogger.Error("GetOnuInfo not implemented")
//	return new(openolt.OnuIndication), nil
//}

func (o *OltDevice) GetPonIf(context context.Context, packet *openolt.Interface) (*openolt.IntfIndication, error) {
	oltLogger.Error("GetPonIf not implemented")
	return new(openolt.IntfIndication), nil
}

func (s *OltDevice) CreateTrafficQueues(context.Context, *tech_profile.TrafficQueues) (*openolt.Empty, error) {
	oltLogger.Info("received CreateTrafficQueues")
	return new(openolt.Empty), nil
}

func (s *OltDevice) RemoveTrafficQueues(_ context.Context, tq *tech_profile.TrafficQueues) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"OnuId":     tq.OnuId,
		"IntfId":    tq.IntfId,
		"OnuPortNo": tq.PortNo,
		"UniId":     tq.UniId,
	}).Info("received RemoveTrafficQueues")
	return new(openolt.Empty), nil
}

func (s *OltDevice) CreateTrafficSchedulers(_ context.Context, trafficSchedulers *tech_profile.TrafficSchedulers) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"OnuId":     trafficSchedulers.OnuId,
		"IntfId":    trafficSchedulers.IntfId,
		"OnuPortNo": trafficSchedulers.PortNo,
		"UniId":     trafficSchedulers.UniId,
	}).Info("received CreateTrafficSchedulers")

	if !s.enablePerf {
		pon, err := s.GetPonById(trafficSchedulers.IntfId)
		if err != nil {
			oltLogger.Errorf("Error retrieving PON by IntfId: %v", err)
			return new(openolt.Empty), err
		}
		onu, err := pon.GetOnuById(trafficSchedulers.OnuId)
		if err != nil {
			oltLogger.Errorf("Error retrieving ONU from pon by OnuId: %v", err)
			return new(openolt.Empty), err
		}
		onu.TrafficSchedulers = trafficSchedulers
	}
	return new(openolt.Empty), nil
}

func (s *OltDevice) RemoveTrafficSchedulers(context context.Context, trafficSchedulers *tech_profile.TrafficSchedulers) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"OnuId":     trafficSchedulers.OnuId,
		"IntfId":    trafficSchedulers.IntfId,
		"OnuPortNo": trafficSchedulers.PortNo,
	}).Info("received RemoveTrafficSchedulers")
	if !s.enablePerf {
		pon, err := s.GetPonById(trafficSchedulers.IntfId)
		if err != nil {
			oltLogger.Errorf("Error retrieving PON by IntfId: %v", err)
			return new(openolt.Empty), err
		}
		onu, err := pon.GetOnuById(trafficSchedulers.OnuId)
		if err != nil {
			oltLogger.Errorf("Error retrieving ONU from pon by OnuId: %v", err)
			return new(openolt.Empty), err
		}

		onu.TrafficSchedulers = nil
	}
	return new(openolt.Empty), nil
}

func (o *OltDevice) PerformGroupOperation(ctx context.Context, group *openolt.Group) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"GroupId": group.GroupId,
		"Command": group.Command,
		"Members": group.Members,
		"Action":  group.Action,
	}).Debug("received PerformGroupOperation")
	return &openolt.Empty{}, nil
}

func (o *OltDevice) DeleteGroup(ctx context.Context, group *openolt.Group) (*openolt.Empty, error) {
	oltLogger.WithFields(log.Fields{
		"GroupId": group.GroupId,
		"Command": group.Command,
		"Members": group.Members,
		"Action":  group.Action,
	}).Debug("received PerformGroupOperation")
	return &openolt.Empty{}, nil
}

func (o *OltDevice) GetExtValue(ctx context.Context, in *openolt.ValueParam) (*extension.ReturnValues, error) {
	return &extension.ReturnValues{}, nil
}

func (o *OltDevice) OnuItuPonAlarmSet(ctx context.Context, in *config.OnuItuPonAlarm) (*openolt.Empty, error) {
	return &openolt.Empty{}, nil
}

func (o *OltDevice) GetLogicalOnuDistanceZero(ctx context.Context, in *openolt.Onu) (*openolt.OnuLogicalDistance, error) {
	return &openolt.OnuLogicalDistance{}, nil
}

func (o *OltDevice) GetLogicalOnuDistance(ctx context.Context, in *openolt.Onu) (*openolt.OnuLogicalDistance, error) {
	return &openolt.OnuLogicalDistance{}, nil
}

func (o *OltDevice) GetPonRxPower(ctx context.Context, in *openolt.Onu) (*openolt.PonRxPowerData, error) {
	return &openolt.PonRxPowerData{}, nil
}

func (o *OltDevice) GetGemPortStatistics(ctx context.Context, in *openolt.OnuPacket) (*openolt.GemPortStatistics, error) {
	return &openolt.GemPortStatistics{}, nil
}

func (o *OltDevice) GetOnuStatistics(ctx context.Context, in *openolt.Onu) (*openolt.OnuStatistics, error) {
	return &openolt.OnuStatistics{}, nil
}

func (o *OltDevice) storeAllocId(flow *openolt.Flow) {
	o.AllocIDsLock.Lock()
	defer o.AllocIDsLock.Unlock()

	if _, ok := o.AllocIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)]; !ok {
		oltLogger.WithFields(log.Fields{
			"IntfId":    flow.AccessIntfId,
			"OnuId":     flow.OnuId,
			"PortNo":    flow.PortNo,
			"GemportId": flow.GemportId,
			"FlowId":    flow.FlowId,
		}).Error("trying-to-store-alloc-id-for-unknown-onu")
	}

	oltLogger.WithFields(log.Fields{
		"IntfId":    flow.AccessIntfId,
		"OnuId":     flow.OnuId,
		"PortNo":    flow.PortNo,
		"GemportId": flow.GemportId,
		"FlowId":    flow.FlowId,
	}).Debug("storing-alloc-id-via-flow")

	if _, ok := o.AllocIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo]; !ok {
		o.AllocIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo] = make(map[int32]map[uint64]bool)
	}
	if _, ok := o.AllocIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo][flow.AllocId]; !ok {
		o.AllocIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo][flow.AllocId] = make(map[uint64]bool)
	}
	o.AllocIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo][flow.AllocId][flow.FlowId] = true
}

func (o *OltDevice) freeAllocId(flow *openolt.Flow) {
	// if this is the last flow referencing the AllocId then remove it
	o.AllocIDsLock.Lock()
	defer o.AllocIDsLock.Unlock()

	oltLogger.WithFields(log.Fields{
		"IntfId":    flow.AccessIntfId,
		"OnuId":     flow.OnuId,
		"PortNo":    flow.PortNo,
		"GemportId": flow.GemportId,
	}).Debug("freeing-alloc-id-via-flow")

	// NOTE look at the freeGemPortId implementation for comments and context
	for ponId, ponValues := range o.AllocIDs {
		for onuId, onuValues := range ponValues {
			for uniId, uniValues := range onuValues {
				for allocId, flows := range uniValues {
					for flowId := range flows {
						// if the flow matches, remove it from the map.
						if flow.FlowId == flowId {
							delete(o.AllocIDs[ponId][onuId][uniId][allocId], flow.FlowId)
						}
						// if that was the last flow for a particular allocId, remove the entire allocId
						if len(o.AllocIDs[ponId][onuId][uniId][allocId]) == 0 {
							delete(o.AllocIDs[ponId][onuId][uniId], allocId)
						}
					}
				}
			}
		}
	}
}

func (o *OltDevice) storeGemPortId(ponId uint32, onuId uint32, portNo uint32, gemId int32, flowId uint64) {
	o.GemPortIDsLock.Lock()
	defer o.GemPortIDsLock.Unlock()

	if _, ok := o.GemPortIDs[ponId][onuId]; !ok {
		oltLogger.WithFields(log.Fields{
			"IntfId":    ponId,
			"OnuId":     onuId,
			"PortNo":    portNo,
			"GemportId": gemId,
			"FlowId":    flowId,
		}).Error("trying-to-store-gemport-for-unknown-onu")
	}

	oltLogger.WithFields(log.Fields{
		"IntfId":    ponId,
		"OnuId":     onuId,
		"PortNo":    portNo,
		"GemportId": gemId,
		"FlowId":    flowId,
	}).Debug("storing-alloc-id-via-flow")

	if _, ok := o.GemPortIDs[ponId][onuId][portNo]; !ok {
		o.GemPortIDs[ponId][onuId][portNo] = make(map[int32]map[uint64]bool)
	}
	if _, ok := o.GemPortIDs[ponId][onuId][portNo][gemId]; !ok {
		o.GemPortIDs[ponId][onuId][portNo][gemId] = make(map[uint64]bool)
	}
	o.GemPortIDs[ponId][onuId][portNo][gemId][flowId] = true
}

func (o *OltDevice) storeGemPortIdByFlow(flow *openolt.Flow) {
	oltLogger.WithFields(log.Fields{
		"IntfId":        flow.AccessIntfId,
		"OnuId":         flow.OnuId,
		"PortNo":        flow.PortNo,
		"GemportId":     flow.GemportId,
		"FlowId":        flow.FlowId,
		"ReplicateFlow": flow.ReplicateFlow,
		"PbitToGemport": flow.PbitToGemport,
	}).Debug("storing-gem-port-id-via-flow")

	if flow.ReplicateFlow {
		for _, gem := range flow.PbitToGemport {
			o.storeGemPortId(uint32(flow.AccessIntfId), uint32(flow.OnuId), flow.PortNo, int32(gem), flow.FlowId)
		}
	} else {
		o.storeGemPortId(uint32(flow.AccessIntfId), uint32(flow.OnuId), flow.PortNo, flow.GemportId, flow.FlowId)
	}
}

func (o *OltDevice) freeGemPortId(flow *openolt.Flow) {
	// if this is the last flow referencing the GemPort then remove it
	o.GemPortIDsLock.Lock()
	defer o.GemPortIDsLock.Unlock()

	oltLogger.WithFields(log.Fields{
		"IntfId":    flow.AccessIntfId,
		"OnuId":     flow.OnuId,
		"PortNo":    flow.PortNo,
		"GemportId": flow.GemportId,
	}).Debug("freeing-gem-port-id-via-flow")

	// NOTE that this loop is not very performant, it would be better if the flow carries
	// the same information that it carries during a FlowAdd. If so we can directly remove
	// items from the map

	//delete(o.GemPortIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo][flow.GemportId], flow.FlowId)
	//if len(o.GemPortIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo][flow.GemportId]) == 0 {
	//	delete(o.GemPortIDs[uint32(flow.AccessIntfId)][uint32(flow.OnuId)][flow.PortNo], flow.GemportId)
	//}

	// NOTE this loop assumes that flow IDs are unique per device
	for ponId, ponValues := range o.GemPortIDs {
		for onuId, onuValues := range ponValues {
			for uniId, uniValues := range onuValues {
				for gemId, flows := range uniValues {
					for flowId := range flows {
						// if the flow matches, remove it from the map.
						if flow.FlowId == flowId {
							delete(o.GemPortIDs[ponId][onuId][uniId][gemId], flow.FlowId)
						}
						// if that was the last flow for a particular gem, remove the entire gem
						if len(o.GemPortIDs[ponId][onuId][uniId][gemId]) == 0 {
							delete(o.GemPortIDs[ponId][onuId][uniId], gemId)
						}
					}
				}
			}
		}
	}
}

// validateFlow checks that:
// - the AllocId is not used in any flow referencing other ONUs/UNIs on the same PON
// - the GemPortId is not used in any flow referencing other ONUs/UNIs on the same PON
func (o *OltDevice) validateFlow(flow *openolt.Flow) error {
	// validate gemPort
	o.GemPortIDsLock.RLock()
	defer o.GemPortIDsLock.RUnlock()
	for onuId, onu := range o.GemPortIDs[uint32(flow.AccessIntfId)] {
		if onuId == uint32(flow.OnuId) {
			continue
		}
		for uniId, uni := range onu {
			for gem := range uni {
				if flow.ReplicateFlow {
					for _, flowGem := range flow.PbitToGemport {
						if gem == int32(flowGem) {
							return fmt.Errorf("gem-%d-already-in-use-on-uni-%d-onu-%d-replicated-flow-%d", gem, uniId, onuId, flow.FlowId)
						}
					}
				} else {
					if gem == flow.GemportId {
						return fmt.Errorf("gem-%d-already-in-use-on-uni-%d-onu-%d-flow-%d", gem, uniId, onuId, flow.FlowId)
					}
				}
			}
		}
	}

	o.AllocIDsLock.RLock()
	defer o.AllocIDsLock.RUnlock()
	for onuId, onu := range o.AllocIDs[uint32(flow.AccessIntfId)] {
		if onuId == uint32(flow.OnuId) {
			continue
		}
		for uniId, uni := range onu {
			for allocId := range uni {
				if allocId == flow.AllocId {
					return fmt.Errorf("allocId-%d-already-in-use-on-uni-%d-onu-%d-flow-%d", allocId, uniId, onuId, flow.FlowId)
				}
			}
		}
	}

	return nil
}

// clearAllResources is invoked up OLT Reboot to remove all the allocated
// GemPorts, AllocId and ONU-IDs across the PONs
func (o *OltDevice) clearAllResources() {

	// remove the resources received via flows
	o.GemPortIDsLock.Lock()
	o.GemPortIDs = make(map[uint32]map[uint32]map[uint32]map[int32]map[uint64]bool)
	o.GemPortIDsLock.Unlock()
	o.AllocIDsLock.Lock()
	o.AllocIDs = make(map[uint32]map[uint32]map[uint32]map[int32]map[uint64]bool)
	o.AllocIDsLock.Unlock()

	// remove the resources received via OMCI
	for _, pon := range o.Pons {
		pon.removeAllAllocIds()
		pon.removeAllGemPorts()
		pon.removeAllOnuIds()
	}
}

func (o *OltDevice) GetVlan(ctx context.Context, request *bossopenolt.BossRequest)(*bossopenolt.GetVlanResponse, error){
	oltLogger.WithFields(log.Fields{
		"request" : request,
	}).Debug("GetVlann......")

	resp := bossopenolt.GetVlanResponse{
		DeviceId : request.DeviceId,
		VlanMode : 0,
		Fields : "0x3064",
	}
	return &resp, nil
}

func(o *OltDevice) GetOltConnect(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.OltConnResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.OltConnResponse{
		DeviceId : reqMessage.DeviceId,
		Ip : "192.168.0.1",
		Mac : "00:AA:10:11:13:03",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetOltDeviceInfo(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.OltDevResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.OltDevResponse{
		DeviceId : reqMessage.DeviceId,
		FpgaType : "25G OLT",
		FpgaVer  : "1.0",
		Fpga_Date : "2020.09.02",
		SwVer : "1.0",
		SwDate : "2020.06.30",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetPmdTxDis(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetPmdTxdis(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.PmdTxdisResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/
//	var Parameter *bossopenolt.SetPmdTxdis = &reqMessage.GetData().GetSetpmtxdisParam()
	response := &bossopenolt.PmdTxdisResponse{
		PortNo : reqMessage.GetParam().GetGetpmdskindParam().PortNo,
		Status : "enable",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetDevicePmdStatus(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.PmdStatusResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.PmdStatusResponse{
		PortNo : reqMessage.GetParam().GetGetpmdskindParam().PortNo,
		Loss : "clear",
		Module : "Inject",
		Fault : "Normal",
		Link : "Down",
	}
	//return response, nil
	return response, nil
}

func(o *OltDevice) SetDevicePort(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetDevicePort(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.GetPortResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.GetPortResponse{
		PortNo : reqMessage.GetParam().GetSetportkindParam().PortNo,
		State : "enable",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) PortReset(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetMtuSize(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetMtuSize(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.MtuSizeResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.MtuSizeResponse{
		Mtu : 1,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetVlan(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetLutMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetLutMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ModeResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ModeResponse{
		DeviceId : reqMessage.DeviceId,
		Mode : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetAgingMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetAgingMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ModeResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ModeResponse{
		DeviceId : reqMessage.DeviceId,
		Mode : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetAgingTime(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetAgingTime(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.AgingTimeResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/
	response := &bossopenolt.AgingTimeResponse{
		DeviceId : reqMessage.DeviceId,
		AgingTime : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetDeviceMacInfo(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.DevMacInfoResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/
	response := &bossopenolt.DevMacInfoResponse{
		DeviceId : reqMessage.DeviceId,
		Mtu : 1522,
		VlanMode : 0,
		AgingMode : 0,
		AgingTime : 10,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetSdnTable(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.SdnTableKeyResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/
	response := &bossopenolt.SdnTableKeyResponse{
		HashKey : 01,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetSdnTable(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.SdnTableResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/
	response := &bossopenolt.SdnTableResponse{
		DeviceId : reqMessage.DeviceId,
		Address : 111,
		PortId : 0,
		Vlan: "0",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetLength(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}

func(o *OltDevice) GetLength(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.LengthResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.LengthResponse{
		DeviceId : reqMessage.DeviceId,
		Value : 0x00,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetQuietZone(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetQuietZone(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.QuietZoneResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.QuietZoneResponse{
		DeviceId : reqMessage.DeviceId,
		Value : 0x00,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetFecMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetFecMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ModeResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ModeResponse{
		DeviceId : reqMessage.DeviceId,
		Mode : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) AddOnu(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.AddOnuResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.AddOnuResponse{
		DeviceId : reqMessage.DeviceId,
		OnuId : reqMessage.GetParam().GetOnuctrlParam().OnuId,
		Result : "success",
		Rate : "25G",
		VendorId : "747421",
		Vssn : "10111001",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) DeleteOnu25G(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) AddOnuSla(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) ClearOnuSla(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetSlaTable(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.RepeatedSlaResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/
	item := &bossopenolt.SlaResponse{
		DeviceId : reqMessage.DeviceId,
		OnuId : 0,
		Tcont : 0,
		Type : "SBDBA",
		Si : 1,
		Abmin :2,
		Absur : 1,
		Fec : "On",
		Distance : 1,
	}
	items:=[]*bossopenolt.SlaResponse{}
	items = append(items, item)
	response := &bossopenolt.RepeatedSlaResponse{
		Resp : items,
	}

	//return response, nil
	return response, nil
}
func(o *OltDevice) SetOnuAllocid(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) DelOnuAllocid(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetOnuVssn(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetOnuVssn(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.OnuVssnResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.OnuVssnResponse{
		DeviceId : reqMessage.DeviceId,
		OnuId : reqMessage.GetParam().GetOnuctrlParam().OnuId,
		Vssn : 0x123,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetOnuDistance(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.OnuDistResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.OnuDistResponse{
		DeviceId : reqMessage.DeviceId,
		OnuId : reqMessage.GetParam().GetOnuctrlParam().OnuId,
		Distance : 1,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetBurstDelimiter(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetBurstDelimiter(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BurstDelimitResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.BurstDelimitResponse{
		DeviceId : reqMessage.DeviceId,
		Length: 0,
		Delimiter : "0x00",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetBurstPreamble(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetBurstPreamble(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BurstPreambleResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.BurstPreambleResponse{
		DeviceId : reqMessage.DeviceId,
		Length: 0,
		Preamble : "0x00",
		Repeat : 80,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetBurstVersion(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetBurstVersion(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BurstVersionResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.BurstVersionResponse{
		DeviceId : reqMessage.DeviceId,
		Version: "1",
		Index : 3,
		Pontag : 0x00000000001,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetBurstProfile(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetBurstProfile(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BurstProfileResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.BurstProfileResponse{
		DeviceId : reqMessage.DeviceId,
		OnuId : reqMessage.GetParam().GetOnuctrlParam().OnuId,
		Version : "3",
		Index : 1,
		DelimiterLength : 4,
		Delimiter : "0xa5465465sdf4d",
		PreambleLength : 8,
		Preamble : "0xaaaaaaa",
		Repeat : 80,
		Pontag : 0x000001,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetRegisterStatus(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.RegisterStatusResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.RegisterStatusResponse{
		DeviceId : reqMessage.DeviceId,
		OnuId: reqMessage.GetParam().GetOnuctrlParam().OnuId,
		Status : "Registered",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetOnuInfo(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.OnuInfoResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.OnuInfoResponse{
		DeviceId : reqMessage.DeviceId,
		OnuId: reqMessage.GetParam().GetOnuctrlParam().OnuId,
		Rate : "25G",
		VendorId : "ETRI",
		Vssn : "00000001",
		Distance : 1,
		Status : "Running",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetOmciStatus(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.StatusResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.StatusResponse{
		DeviceId : reqMessage.DeviceId,
		Status : "full",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetDsOmciOnu(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetDsOmciData(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetUsOmciData(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.OmciDataResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.OmciDataResponse{
		DeviceId: reqMessage.DeviceId,
		Control : 0x06,
		Data : 0x08,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetTod(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetTod(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.TodResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.TodResponse{
		DeviceId: reqMessage.DeviceId,
		Mode : 0,
		Time : 10,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetDataMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetDataMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ModeResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ModeResponse{
		DeviceId: reqMessage.DeviceId,
		Mode : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetFecDecMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetFecDecMode(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ModeResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ModeResponse{
		DeviceId: reqMessage.DeviceId,
		Mode : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetDelimiter(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetDelimiter(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.FecDecResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.FecDecResponse{
		DeviceId: reqMessage.DeviceId,
		Value : "0xa15as6",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetErrorPermit(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetErrorPermit(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ErrorPermitResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ErrorPermitResponse{
		DeviceId: reqMessage.DeviceId,
		Value : 3,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetPmControl(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetPmControl(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.PmControlResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.PmControlResponse{
		DeviceId: reqMessage.DeviceId,
		Action :"Dynamic power management cotrol",
		OnuMode : "cyclic sleep mode supported",
		Transinit : 0,
		Txinit : 1,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) GetPmTable(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.PmTableResponse, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.PmTableResponse{
		DeviceId: reqMessage.DeviceId,
		OnuId : reqMessage.GetParam().GetOnuctrlParam().OnuId,
		Mode : "disable",
		Sleep : 0,
		Aware : 0,
		Rxoff : 0,
		Hold : 0,
		Action :"Dynamic power management cotrol",
		Status : "cyclic sleep mode supported",
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetSAOn(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetSAOff(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
	/*response :=&bossopenolt.GetVlanResponse{
		DeviceId : reqMessage.DeviceId,
		VlanMode : 1,
		Fields : "0x3064",
	}*/

	response := &bossopenolt.ExecResult{
		Result : 0,
	}
	//return response, nil
	return response, nil
}
func(o *OltDevice) SetSliceBw(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/

        response := &bossopenolt.ExecResult{
                Result : 0,
        }
        //return response, nil
        return response, nil
}
func(o *OltDevice) GetSliceBw(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.GetSliceBwResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/

        response := &bossopenolt.GetSliceBwResponse{
		DeviceId : reqMessage.DeviceId,
		Bw : 10,
        }
        //return response, nil
        return response, nil
}
func(o *OltDevice) SetSlaV2(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.RepeatedSlaV2Response, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/

   response := &bossopenolt.SlaV2Response{
		DeviceId: reqMessage.DeviceId,
		OnuId : 1,
		Tcont : 1,
		AllocId : "allocId",
		Slice : 1,
		Bw : 1,
		Dba : "SD_",
		Type : "aa",
		Fixed : 1,
		Assur : 2,
		Nogur : 1,
		Max :1,
		Reach : 1.1,
   }
   items := []*bossopenolt.SlaV2Response{}
   items = append(items, response)
   responses := &bossopenolt.RepeatedSlaV2Response{
      Resp : items,
   }
        //return response, nil
        return responses, nil
}
func(o *OltDevice) GetSlaV2(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.RepeatedSlaV2Response, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   response := &bossopenolt.SlaV2Response{
		DeviceId: reqMessage.DeviceId,
		OnuId : 1,
		Tcont : 1,
		AllocId : "allocId",
		Slice : 1,
		Bw : 1,
		Dba : "SD_",
		Type : "aa",
		Fixed : 1,
		Assur : 2,
		Nogur : 1,
		Max :1,
		Reach : 1.1,
   }
   items := []*bossopenolt.SlaV2Response{}
   items = append(items, response)
   responses := &bossopenolt.RepeatedSlaV2Response{
      Resp : items,
   }
        //return response, nil
        return responses, nil
}
func(o *OltDevice) SendOmciData(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BossOmciResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
         response := &bossopenolt.BossOmciResponse{
		DeviceId: reqMessage.DeviceId,
		OnuId : 1,
		OmciData: "BossOmciResponse",
	}
        //return response, nil
        return response, nil
}
func(o *OltDevice) GetPktInd(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BossPktIndResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   response := &bossopenolt.BossPktIndResponse{
		DeviceId: reqMessage.DeviceId,
    Result : "success",
	}
        //return response, nil
        return response, nil
}

func(o *OltDevice) SetLatencyClear(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BossLatencyClearResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   response := &bossopenolt.BossLatencyClearResponse{
		DeviceId: reqMessage.DeviceId,
    Pon : 0,
    Result : 0,
	}
        //return response, nil
        return response, nil
}
func(o *OltDevice) SetLatencyFlow(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BossLatencyFlowResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   response := &bossopenolt.BossLatencyFlowResponse{
		DeviceId: reqMessage.DeviceId,
    Pon : 0,
    XgemId : 0,
	}
        //return response, nil
        return response, nil
}
func(o *OltDevice) GetLatencyFlow(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BossLatencyFlowResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   response := &bossopenolt.BossLatencyFlowResponse{
		DeviceId: reqMessage.DeviceId,
    Pon : 0,
    XgemId : 0,
	}
        //return response, nil
        return response, nil
}
func(o *OltDevice) GetLatencyData(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BossLatencyDataResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   latencyFlag =true
   response := &bossopenolt.BossLatencyDataResponse{
		DeviceId: reqMessage.DeviceId,
    Pon : 0,
    AllocId :0,
    PortId :0,
    Latency: 0,
	}
        //return response, nil
        return response, nil
}
func(o *OltDevice) GetLatencyMeasure(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.BossLatencyMeasureResponse, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   response := &bossopenolt.BossLatencyMeasureResponse{
		DeviceId: reqMessage.DeviceId,
    Pon : 0,
    Measure :0,
	}
        //return response, nil
        return response, nil
}
func(o *OltDevice) GetPortStats(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.ExecResult, error){
        /*response :=&bossopenolt.GetVlanResponse{
                DeviceId : reqMessage.DeviceId,
                VlanMode : 1,
                Fields : "0x3064",
        }*/
   response := &bossopenolt.ExecResult{
    Result :0,
	}
        //return response, nil
        return response, nil
}
//func(o *OltDevice) GetOnuInfo(ctx context.Context, reqMessage *bossopenolt.BossRequest) (*bossopenolt.OnuInfoResponse, error){
//        /*response :=&bossopenolt.GetVlanResponse{
//                DeviceId : reqMessage.DeviceId,
//                VlanMode : 1,
//                Fields : "0x3064",
//        }*/
//   response := &bossopenolt.OnuInfoResponse{
//     DeviceId : reqMessage.DeviceId,
//     OnuId : 1,
//     Rate: "2233",
//     VendorId: "ETRI",
//     Vssn: "VSSN",
//     Distance : 100,
//     Status : "Running",
//	}
//        //return response, nil
//        return response, nil
//}
//
