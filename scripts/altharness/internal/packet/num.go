package packet

import "strconv"

func parseInt64(text string) (int64, error) {
	return strconv.ParseInt(text, 10, 64)
}

func strconvParseFloat(text string) (float64, error) {
	return strconv.ParseFloat(text, 64)
}
