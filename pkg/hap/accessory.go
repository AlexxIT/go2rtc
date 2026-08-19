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

// minLegacyIID is the smallest IID the positional ANSSSCCC layout can produce:
// AID and the service instance are both at least 1, and the smallest service
// type in use is two hex digits. Sequentially allocated IIDs are counted up
// from 1 and so can never reach it, which is what lets both schemes coexist in
// one accessory.
const minLegacyIID = 0x11000000

// InitIID assigns instance IDs. Types of three hex digits or fewer keep the
// positional layout go2rtc has always published, because controllers cache the
// accessory database and renumbering invalidates every existing pairing. The
// four hex digit types added by the HomeKit Secure Video open source spec do
// not fit that layout and are numbered sequentially instead, so adding them to
// an accessory leaves the IIDs of its existing services untouched.
func (a *Accessory) InitIID() {
	var seq uint64
	serviceN := map[string]byte{}

	for _, service := range a.Services {
		n := serviceN[service.Type] + 1
		serviceN[service.Type] = n

		legacy := fitsLegacyIID(service.Type) && n <= 15
		if legacy {
			// ServiceID = ANSSS000
			s := fmt.Sprintf("%x%x%03s000", a.AID, n, service.Type)
			service.IID, _ = strconv.ParseUint(s, 16, 64)
		} else {
			seq++
			service.IID = seq
		}

		for _, character := range service.Characters {
			// A sequentially numbered service has no positional base to offset
			// its characteristics from, so they follow it.
			if legacy && fitsLegacyIID(character.Type) {
				// CharacterID = ANSSSCCC
				character.IID, _ = strconv.ParseUint(character.Type, 16, 64)
				character.IID += service.IID
			} else {
				seq++
				character.IID = seq
			}
		}
	}

	if seq >= minLegacyIID {
		panic("hap: too many sequential IIDs")
	}
}

func fitsLegacyIID(typ string) bool {
	return len(typ) <= 3
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
	Desc string `json:"description,omitempty"`

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
