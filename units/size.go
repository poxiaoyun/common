package units

import "fmt"

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
	TB = 1024 * GB
	PB = 1024 * TB
)

var binaryAbbrs = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}

func getSizeAndUnit(size float64, base float64, _map []string) (float64, string) {
	i := 0
	unitsLimit := len(_map) - 1
	for size >= base && i < unitsLimit {
		size = size / base
		i++
	}
	return size, _map[i]
}

func HumanSize(size float64) string {
	return HumanSizeWithPrecision(size, 3)
}

func HumanSizeWithPrecision(size float64, precision int) string {
	size, unit := getSizeAndUnit(size, 1024.0, binaryAbbrs)
	return fmt.Sprintf("%.*g%s", precision, size, unit)
}
