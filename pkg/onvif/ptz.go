package onvif

const (
	PTZGetConfigurations = "GetConfigurations"
	PTZGetConfiguration  = "GetConfiguration"
	PTZContinuousMove    = "ContinuousMove"
	PTZStop              = "Stop"
	PTZGetStatus         = "GetStatus"
)

// PTZRequest sends a PTZ command to the camera
func (c *Client) PTZRequest(operation string, params ...string) ([]byte, error) {
	var body string
	switch operation {
	case PTZContinuousMove:
		if len(params) < 4 {
			return nil, nil
		}

		//PanTilt velocity
		velocity := `<tt:PanTilt x="` + params[1] + `" y="` + params[2] + `"/>`

		// Add Zoom velocity only if camera supports zoom
		if c.hasZoom {
			velocity += `<tt:Zoom x="` + params[3] + `"/>`
		}

		body = `<tptz:ContinuousMove xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
			<tptz:ProfileToken>` + params[0] + `</tptz:ProfileToken>
			<tptz:Velocity>
				` + velocity + `
			</tptz:Velocity>
		</tptz:ContinuousMove>`
	case PTZStop:
		if len(params) < 1 {
			return nil, nil
		}
		body = `<tptz:Stop xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
			<tptz:ProfileToken>` + params[0] + `</tptz:ProfileToken>
		</tptz:Stop>`
	default:
		body = `<tptz:` + operation + `/>`
	}

	return c.Request(c.ptzURL, body)
}
