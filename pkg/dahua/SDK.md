## 4.3 Audio

## 4.3.3 Post Audio Stream

Post audio stream to device, request is very long and client continues to send audio data. If client want to stop, just close the connection.

|                                                                                                  |                                                    |            |                                                                                                                                                                                                                        |
| ------------------------------------------------------------------------------------------------ | -------------------------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| <b>Request URL</b>                                                                               | http://<server>/cgi-bin/audio.cgi?action=postAudio |            |                                                                                                                                                                                                                        |
| <b>Method</b>                                                                                    | POST                                               |            |                                                                                                                                                                                                                        |
| <b>Request Params ( key=value format in URL )</b>                                                |                                                    |            |                                                                                                                                                                                                                        |
| <b>Name</b>                                                                                      | <b>Type</b>                                        | <b>R/O</b> | <b>Description</b>                                                                                                                                                                                                     |
| httptype                                                                                         | string                                             | R          | audio http transmit format, can be :<br>singlepart: HTTP content is a continuous flow of audio packets<br>multipart: HTTP content type is multipart/x-mixed-replace, and each audio packet ends with a boundary string |
| channel                                                                                          | int                                                | R          | The audio channel index which starts from 1.                                                                                                                                                                           |
| <b>Request Example ( singlepart )</b>                                                            |                                                    |            |                                                                                                                                                                                                                        |
| POST http://192.168.1.108/cgi-bin/audio.cgi?action=postAudio&httptype=singlepart&channel=1 HTTP/ |                                                    |            |                                                                                                                                                                                                                        |

```
1.1
User-Agent: client/1.0
Content-Type: Audio/G.711A
Content-Length: 999999
```

```
<Audio data>
<Audio data>
```

```
...
```

#### Request Example ( multipart )

```
POST http://192.168.1.108/cgi-bin/audio.cgi?action=postAudio&httpType=multipart&channel=1 HTTP/
1.1
User-Agent: client/1.0
Content-Type: multipart/x-mixed-replace; boundary=<boundary>

--<boundary>
Content-Type: Audio/G.711A
Content-Length: 800

<Audio data>
--<boundary>
Content-Type: Audio/G.711A
Content-Length: 800

<Audio data>
--<boundary>
...
```

#### Response Params ( N/A )

| Name   | Type   | R/O   | Description   | Example   |
| ------ | ------ | ----- | ------------- | --------- |
| ------ | ------ | ----- | ------------- | --------- |

#### Response Example

( N/A )

#### Appendix A: Audio Encode Type

| MIME ( Content-Type ) | Description  |
| --------------------- | ------------ |
| Audio/PCM             | PCM          |
| Audio/ADPCM           | ADPCM        |
| Audio/G.711A          | G.711 A Law  |
| Audio/G.711Mu         | G.711 Mu Law |
| Audio/G.726           | G.726        |
| Audio/G.729           | G.729        |
| Audio/MPEG2           | MPEG2        |
| Audio/AMR             | AMR          |
| Audio/AAC             | AAC          |
