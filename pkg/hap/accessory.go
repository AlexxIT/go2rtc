package hap

import (
	"fmt"
	"strconv"
)

const (
	FormatString = "string"
	FormatBool   = "bool"
	FormatFloat  = "float"
	FormatUInt8  = "uint8"
	FormatUInt16 = "uint16"
	FormatUInt32 = "uint32"
	FormatInt32  = "int32"
	FormatUInt64 = "uint64"
	FormatData   = "data"
	FormatTLV8   = "tlv8"

	UnitPercentage = "percentage"
)

var PR = []string{"pr"}
var PW = []string{"pw"}
var PRPW = []string{"pr", "pw"}
var EVPRPW = []string{"ev", "pr", "pw"}
var EVPR = []string{"ev", "pr"}

type Accessory struct {
	AID      uint8      `json:"aid"` // 150 unique accessories per bridge
	Services []*Service `json:"services"`
}

func (a *Accessory) InitIID() {
	serviceN := map[string]byte{}
	for _, service := range a.Services {
		if len(service.Type) > 3 {
			panic(service.Type)
		}

		n := serviceN[service.Type] + 1
		serviceN[service.Type] = n

		if n > 15 {
			panic(n)
		}

		// ServiceID   = ANSSS000
		s := fmt.Sprintf("%x%x%03s000", a.AID, n, service.Type)
		service.IID, _ = strconv.ParseUint(s, 16, 64)

		for _, character := range service.Characters {
			if len(character.Type) > 3 {
				panic(character.Type)
			}

			// CharacterID = ANSSSCCC
			character.IID, _ = strconv.ParseUint(character.Type, 16, 64)
			character.IID += service.IID
		}
	}
}

func (a *Accessory) GetService(servType string) *Service {
	for _, serv := range a.Services {
		if serv.Type == servType {
			return serv
		}
	}
	return nil
}

func (a *Accessory) GetCharacter(charType string) *Character {
	for _, serv := range a.Services {
		for _, char := range serv.Characters {
			if char.Type == charType {
				return char
			}
		}
	}
	return nil
}

func (a *Accessory) GetCharacterByID(iid uint64) *Character {
	for _, serv := range a.Services {
		for _, char := range serv.Characters {
			if char.IID == iid {
				return char
			}
		}
	}
	return nil
}

type Service struct {
	Type       string       `json:"type"`
	IID        uint64       `json:"iid"`
	Primary    bool         `json:"primary,omitempty"`
	Characters []*Character `json:"characteristics"`
	Linked     []int        `json:"linked,omitempty"`
}

func (s *Service) GetCharacter(charType string) *Character {
	for _, char := range s.Characters {
		if char.Type == charType {
			return char
		}
	}
	return nil
}

func ServiceAccessoryInformation(manuf, model, name, serial, firmware string) *Service {
	return &Service{
		Type: "3E", // AccessoryInformation
		Characters: []*Character{
			{
				Type:   "14",
				Format: FormatBool,
				Perms:  PW,
				//Descr:  "Identify",
			}, {
				Type:   "20",
				Format: FormatString,
				Value:  manuf,
				Perms:  PR,
				//Descr:  "Manufacturer",
				//MaxLen: 64,
			}, {
				Type:   "21",
				Format: FormatString,
				Value:  model,
				Perms:  PR,
				//Descr:  "Model",
				//MaxLen: 64,
			}, {
				Type:   "23",
				Format: FormatString,
				Value:  name,
				Perms:  PR,
				//Descr:  "Name",
				//MaxLen: 64,
			}, {
				Type:   "30",
				Format: FormatString,
				Value:  serial,
				Perms:  PR,
				//Descr:  "Serial Number",
				//MaxLen: 64,
			}, {
				Type:   "52",
				Format: FormatString,
				Value:  firmware,
				Perms:  PR,
				//Descr:  "Firmware Revision",
			},
		},
	}
}

func ServiceHAPProtocolInformation() *Service {
	return &Service{
		Type: "A2", // 'HAPProtocolInformation'
		Characters: []*Character{
			{
				Type:   "37",
				Format: FormatString,
				Value:  "1.1.0",
				Perms:  PR,
				//Descr:  "Version",
				//MaxLen: 64,
			},
		},
	}
}
