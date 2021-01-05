package main

import (
	"flag"
	"fmt"
	"math/big"
	"regexp"

	"github.com/dfuse-io/tooling/cli"
)

var reversedFlag = flag.Bool("r", false, "Decode assuming the input value is a reverted number")

func main() {
	flag.Parse()

	scanner := cli.NewArgumentScanner()
	for element, ok := scanner.ScanArgument(); ok; element, ok = scanner.ScanArgument() {
		fmt.Println(toDec(element))
	}
}

var scientificNotationRegexp = regexp.MustCompile(`^([0-9]+)?\.[0-9]+(e|E)\+[0-9]+$`)

func toDec(element string) string {
	if cli.HexRegexp.MatchString(element) {
		value, err := cli.DecodeHex(element)
		cli.NoError(err, "invalid number %q", element)

		bigValue := new(big.Int).SetBytes(value)

		if *reversedFlag && bigValue.BitLen() > 0 {
			max := new(big.Int).Lsh(big.NewInt(1), uint(bigValue.BitLen()-1))
			for i := 0; i < bigValue.BitLen(); i++ {
				max.SetBit(max, i, 1)
			}

			bigValue = new(big.Int).Sub(max, bigValue)
		}

		return bigValue.String()
	}

	if scientificNotationRegexp.MatchString(element) {
		flt, _, err := big.ParseFloat(element, 10, 0, big.ToNearestEven)
		cli.NoError(err, "invalid scientific notation %q", element)

		bigValue, _ := flt.Int(new(big.Int))
		return bigValue.String()
	}

	return element
}
