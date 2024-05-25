## Logs

```
request  []interface {}{"connect", 1, map[string]interface {}{"app":"s", "flashVer":"FMLE/3.0 (compatible; FMSc/1.0)", "tcUrl":"rtmps://xxx.rtmp.t.me/s/xxxxx"}}
response []interface {}{"_result", 1, map[string]interface {}{"capabilities":31, "fmsVer":"FMS/3,0,1,123"}, map[string]interface {}{"code":"NetConnection.Connect.Success", "description":"Connection succeeded.", "level":"status", "objectEncoding":0}}
request  []interface {}{"releaseStream", 2, interface {}(nil), "xxxxx"}
request  []interface {}{"FCPublish", 3, interface {}(nil), "xxxxx"}
request  []interface {}{"createStream", 4, interface {}(nil)}
response []interface {}{"_result", 2, interface {}(nil)}
response []interface {}{"_result", 4, interface {}(nil), 1}
request  []interface {}{"publish", 5, interface {}(nil), "xxxxx", "live"}
response []interface {}{"onStatus", 0, interface {}(nil), map[string]interface {}{"code":"NetStream.Publish.Start", "description":"xxxxx is now published", "detail":"xxxxx", "level":"status"}}
```

## Useful links

- https://en.wikipedia.org/wiki/Flash_Video
- https://en.wikipedia.org/wiki/Real-Time_Messaging_Protocol
- https://rtmp.veriskope.com/pdf/rtmp_specification_1.0.pdf
- https://rtmp.veriskope.com/docs/spec/
