package tuya

import (
	"crypto/md5"
	"fmt"
)

func calTokenSign(ts int64) string {
	data := fmt.Sprintf("%s%s%d", App.ClientID, App.Secret, ts)

	val := md5.Sum([]byte(data))

	// md5值转换为大写
	res := fmt.Sprintf("%X", val)
	return res
}

func calBusinessSign(ts int64) string {
	data := fmt.Sprintf("%s%s%s%d", App.ClientID, App.AccessToken, App.Secret, ts)

	val := md5.Sum([]byte(data))

	// md5值转换为大写
	res := fmt.Sprintf("%X", val)
	return res
}
