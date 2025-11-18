package setup

import (
	"strconv"
	"strings"
)

const (
	FlagNFC = 1
	FlagIP  = 2
	FlagBLE = 4
	FlagWAC = 8 // Wireless Accessory Configuration (WAC)/Apples MFi
)

func GenerateSetupURI(category, pin, setupID string) string {
	c, _ := strconv.Atoi(category)
	p, _ := strconv.Atoi(strings.ReplaceAll(pin, "-", ""))
	payload := int64(c&0xFF)<<31 | int64(FlagIP&0xF)<<27 | int64(p&0x7FFFFFF)
	return "X-HM://" + FormatInt36(payload, 9) + setupID
}

// FormatInt36 equal to strings.ToUpper(fmt.Sprintf("%0"+strconv.Itoa(n)+"s", strconv.FormatInt(value, 36)))
func FormatInt36(value int64, n int) string {
	b := make([]byte, n)
	for i := n - 1; 0 <= i; i-- {
		b[i] = digits[value%36]
		value /= 36
	}
	return string(b)
}

const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
