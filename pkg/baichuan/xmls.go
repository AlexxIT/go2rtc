package baichuan

// Baichuan XML templates adapted from reolink_aio

const (
	xmlHeader = `<?xml version="1.0" encoding="UTF-8" ?>` + "\n"

	dingDongOpt1XML = xmlHeader + `<body>
<dingdongDeviceOpt version="1.1">
<opt>delDevice</opt>
<id>%d</id>
</dingdongDeviceOpt>
</body>`

	dingDongOpt2XML = xmlHeader + `<body>
<dingdongDeviceOpt version="1.1">
<id>%d</id>
<opt>getParam</opt>
</dingdongDeviceOpt>
</body>`

	dingDongOpt3XML = xmlHeader + `<body>
<dingdongDeviceOpt version="1.1">
<opt>setParam</opt>
<id>%d</id>
<volLevel>%d</volLevel>
<ledState>%d</ledState>
<name>%s</name>
</dingdongDeviceOpt>
</body>`

	dingDongOpt4XML = xmlHeader + `<body>
<dingdongDeviceOpt version="1.1">
<id>%d</id>
<opt>ringWithMusic</opt>
<musicId>%d</musicId>
</dingdongDeviceOpt>
</body>`

	setDingDongCfgXML = xmlHeader + `<body>
<dingdongCfg version="1.1">
<deviceCfg>
<id>%d</id>
<alarminCfg>
<valid>%d</valid>
<musicId>%d</musicId>
<type>%s</type>
</alarminCfg>
</deviceCfg>
</dingdongCfg>
</body>`

	dingDongSilentXML = xmlHeader + `<body>
<dingdongSilentMode version="1.1">
<id>%d</id>
</dingdongSilentMode>
</body>`

	setDingDongSilentXML = xmlHeader + `<body>
<dingdongSilentMode version="1.1">
<id>%d</id>
<time>%d</time>
<type>63</type>
</dingdongSilentMode>
</body>`

	getDingDongCtrlXML = xmlHeader + `<body>
<dingdongCtrl version="1.1">
<opt>machineStateGet</opt>
</dingdongCtrl>
</body>`

	setDingDongCtrlXML = xmlHeader + `<body>
<dingdongCtrl version="1.1">
<opt>machineStateSet</opt>
<type>%d</type>
<bopen>%d</bopen>
<bsave>1</bsave>
<time>%d</time>
</dingdongCtrl>
</body>`

	quickReplyPlayXML = xmlHeader + `<body>
<audioFileInfo version="1.1">
<channelId>%d</channelId>
<id>%d</id>
<timeout>0</timeout>
</audioFileInfo>
</body>`

	setPrivacyModeXML = xmlHeader + `<body>
<sleepState version="1.1">
<operate>2</operate>
<sleep>%d</sleep>
</sleepState>
</body>`

	getSceneInfoXML = xmlHeader + `<body>
<sceneCfg version="1.1">
<id>%d</id>
</sceneCfg>
</body>`

	disableSceneXML = xmlHeader + `<body>
<sceneModeCfg version="1.1">
<enable>0</enable>
</sceneModeCfg>
</body>`

	setSceneXML = xmlHeader + `<body>
<sceneModeCfg version="1.1">
<enable>1</enable>
<curSceneId>%d</curSceneId>
</sceneModeCfg>
</body>`

	setWhiteLedXML = xmlHeader + `<body>
<FloodlightManual version="1.1">
<channelId>%d</channelId>
<status>%d</status>
<duration>180</duration>
</FloodlightManual>
</body>`

	wifiSSIDXML = xmlHeader + `<body>
<Wifi version="1.1">
<scanAp>0</scanAp>
</Wifi>
</body>`

	sirenManualXML = xmlHeader + `<body>
<audioPlayInfo version="1.1">
<channelId>%d</channelId>
<playMode>2</playMode>
<playDuration>10</playDuration>
<playTimes>1</playTimes>
<onOff>%d</onOff>
</audioPlayInfo>
</body>`

	sirenTimesXML = xmlHeader + `<body>
<audioPlayInfo version="1.1">
<channelId>%d</channelId>
<playMode>0</playMode>
<playDuration>10</playDuration>
<playTimes>%d</playTimes>
<onOff>1</onOff>
</audioPlayInfo>
</body>`

	sirenHubManualXML = xmlHeader + `<body>
<audioPlayInfo version="1.1">
<playMode>2</playMode>
<playDuration>10</playDuration>
<playTimes>1</playTimes>
<onOff>%d</onOff>
</audioPlayInfo>
</body>`

	sirenHubTimesXML = xmlHeader + `<body>
<audioPlayInfo version="1.1">
<playMode>0</playMode>
<playDuration>10</playDuration>
<playTimes>%d</playTimes>
<onOff>1</onOff>
</audioPlayInfo>
</body>`

	setAutoFocusXML = xmlHeader + `<body>
<AutoFocus version="1.1">
<channelId>%d</channelId>
<disable>%d</disable>
</AutoFocus>
</body>`

	getAiAlarmXML = xmlHeader + `<body>
<AiDetectCfg version="1.1">
<chn>%d</chn>
<type>%s</type>
</AiDetectCfg>
</body>`

	snapXML = xmlHeader + `<body>
<Snap version="1.1">
<channelId>%d</channelId>
<logicChannel>%d</logicChannel>
<time>0</time>
<fullFrame>0</fullFrame>
<streamType>%s</streamType>
</Snap>
</body>`

	ptzControlXML = xmlHeader + `<body>
<PtzControl version="1.1">
<channelId>%d</channelId>
<command>%s</command>
<speed>%d</speed>
</PtzControl>
</body>`

	ptzPresetXML = xmlHeader + `<body>
<PtzPreset version="1.1">
<channelId>%d</channelId>
<presetList>
<preset>
<id>%d</id>
<command>toPos</command>
</preset>
</presetList>
</PtzPreset>
</body>`

	ptz3DLocationXML = xmlHeader + `<body>
<Ptz3DLocation version="1.1">
<channelId>%d</channelId>
<posX>%d</posX>
<posY>%d</posY>
<posWidth>%d</posWidth>
<posHeight>%d</posHeight>
<speed>%d</speed>
<width>%d</width>
<height>%d</height>
</Ptz3DLocation>
</body>`

	ptzGuardXML = xmlHeader + `<body>
<PtzGuard version="1.1">
<channelId>%d</channelId>
<benable>%d</benable>
<command>%s</command>
<timeout>%d</timeout>
<needSetPos>%d</needSetPos>
<imageName></imageName>
</PtzGuard>
</body>`
)

// Keep templates around for future expansion and satisfy golangci-lint
var (
	_ = dingDongOpt1XML
	_ = dingDongOpt2XML
	_ = dingDongOpt3XML
	_ = setDingDongCfgXML
	_ = dingDongSilentXML
	_ = getDingDongCtrlXML
	_ = setDingDongCtrlXML
	_ = getSceneInfoXML
	_ = disableSceneXML
	_ = setSceneXML
	_ = wifiSSIDXML
	_ = getAiAlarmXML
	_ = snapXML
	_ = ptzControlXML
	_ = ptzPresetXML
)
