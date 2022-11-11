package hap

type Accessory struct {
	AID      int        `json:"aid"`
	Services []*Service `json:"services"`
}

type Accessories struct {
	Accessories []*Accessory `json:"accessories"`
}

type Characters struct {
	Characters []*Character `json:"characteristics"`
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

func (a *Accessory) GetCharacterByID(iid int) *Character {
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
	IID         int          `json:"iid"`
	Type        string       `json:"type"`
	Primary     bool         `json:"primary,omitempty"`
	Hidden      bool         `json:"hidden,omitempty"`
	Characters  []*Character `json:"characteristics"`
}

func (s *Service) GetCharacter(charType string) *Character {
	for _, char := range s.Characters {
		if char.Type == charType {
			return char
		}
	}
	return nil
}
