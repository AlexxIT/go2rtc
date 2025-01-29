package mjpeg

const (
	markerSOF = 0xC0 // Start Of Frame (Baseline Sequential)
	markerSOI = 0xD8 // Start Of Image
	markerEOI = 0xD9 // End Of Image
	markerSOS = 0xDA // Start Of Scan
	markerDQT = 0xDB // Define Quantization Table
	markerDHT = 0xC4 // Define Huffman Table
)
