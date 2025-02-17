package cli

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
const encodeURL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

var Base64URLRegexp = regexp.MustCompile(`^[a-zA-z0-9\-_]+(=){0,2}$`)
var Base64StdRegexp = regexp.MustCompile(`^[a-zA-z0-9\+\/]+(=){0,2}$`)
var DecRegexp = regexp.MustCompile(`^[0-9]+$`)
var HexRegexp = regexp.MustCompile(`^(0(x|X))?[a-fA-F0-9]+$`)
var SpacesRegexp = regexp.MustCompile(`\s`)

func Ensure(condition bool, message string, args ...interface{}) {
	if !condition {
		NoError(fmt.Errorf(message, args...), "invalid arguments")
	}
}

func NoError(err error, message string, args ...interface{}) {
	if err != nil {
		Quit(message+": "+err.Error(), args...)
	}
}

func Quit(message string, args ...interface{}) {
	fmt.Printf(message+"\n", args...)
	os.Exit(1)
}

func End(message string, args ...interface{}) {
	fmt.Printf(message+"\n", args...)
	os.Exit(0)
}

func EncodeHex(in []byte) string {
	hex := hex.EncodeToString(in)
	if len(hex)%2 == 1 {
		hex = "0" + hex
	}

	return hex
}

func DecodeHex(in string) ([]byte, error) {
	out := strings.TrimPrefix(strings.ToLower(in), "0x")
	if len(out)%2 != 0 {
		out = "0" + out
	}

	return hex.DecodeString(out)
}

type ArgumentScanner interface {
	ScanArgument() (value string, ok bool)
}

func NewArgumentScanner() ArgumentScanner {
	fi, err := os.Stdin.Stat()
	NoError(err, "unable to stat stdin")

	if (fi.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		// Let's allow token as long as 50MiB
		scanner.Buffer(nil, 50*1024*1024)

		return (*bufioArgumentScanner)(scanner)
	}

	args := os.Args[1:]
	if flag.Parsed() {
		args = flag.Args()
	}

	slice := stringSliceArgumentScanner(args)
	return &slice
}

type bufioArgumentScanner bufio.Scanner

func (s *bufioArgumentScanner) ScanArgument() (string, bool) {
	scanner := (*bufio.Scanner)(s)
	ok := scanner.Scan()
	if ok {
		return scanner.Text(), true
	}

	if err := scanner.Err(); err != nil {
		if err == io.EOF {
			return "", false
		}

		NoError(err, "unable to scan argument from reader")
		return "", false
	}

	return "", false
}

type stringSliceArgumentScanner []string

func (s *stringSliceArgumentScanner) ScanArgument() (string, bool) {
	slice := *s
	if len(*s) == 0 {
		return "", false
	}

	value := slice[0]
	*s = slice[1:]
	return value, true
}

// ProcessStandardInputBytes reads standard input using a buffer as big as `bufferSize`
// and pass the read bytes to `processor` function. The number of bytes received by the
// `processor` function might be lower than buffer size but will never be bigger than it.
func ProcessStandardInputBytes(bufferSize int, processor func(bytes []byte)) {
	fi, err := os.Stdin.Stat()
	NoError(err, "unable to stat stdin")
	Ensure((fi.Mode()&os.ModeCharDevice) == 0, "Standard input must be piped when from stdin mode is used")

	reader := bufio.NewReader(os.Stdin)

	buf := make([]byte, bufferSize)
	for {
		n, err := reader.Read(buf)
		if err == io.EOF {
			Ensure(n == 0, "Byte count should be 0 when getting EOF")
			break
		}

		NoError(err, "unable to read %d bytes stdin", bufferSize)
		processor(buf[0:n])
	}
}

func ErrorUsage(usage func() string, message string, args ...interface{}) string {
	return fmt.Sprintf(message+"\n\n"+usage(), args...)
}

func SetupFlag(usage func() string) {
	flag.CommandLine.Usage = func() {
		fmt.Print(usage())
	}
	flag.Parse()
}

func FlagUsage() string {
	buf := bytes.NewBuffer(nil)
	oldOutput := flag.CommandLine.Output()
	defer func() { flag.CommandLine.SetOutput(oldOutput) }()

	flag.CommandLine.SetOutput(buf)
	flag.CommandLine.PrintDefaults()

	return buf.String()
}

func AskForConfirmation(message string, args ...interface{}) bool {
	for {
		fmt.Printf(message+" ", args...)
		var response string
		_, err := fmt.Scanln(&response)
		if err != nil {
			panic(fmt.Errorf("unable to read user confirmation, are you in an interactive terminal: %w", err))
		}

		response = strings.ToLower(strings.TrimSpace(response))
		for _, yesResponse := range []string{"y", "yes"} {
			if response == yesResponse {
				return true
			}
		}

		for _, noResponse := range []string{"n", "no"} {
			if response == noResponse {
				return false
			}
		}

		fmt.Println("Only Yes or No accepted, please retry!")
		fmt.Println()
	}
}

func ReadIntegerToBytes(in string) []byte {
	value := new(big.Int)
	value, success := value.SetString(in, 10)
	Ensure(success, "number %q is invalid", in)

	return value.Bytes()
}

//go:generate go-enum -f=$GOFILE --marshal --names

//
// ENUM(
//   None
//   UnixSeconds
//   UnixMilliseconds
// )
//
type DateLikeHint uint

//
// ENUM(
//   Layout
//   Timestamp
// )
//
type DateParsedFrom uint

var _, localOffset = time.Now().Zone()

func ParseDateLikeInput(element string, hint DateLikeHint) (out time.Time, parsedFrom DateParsedFrom, ok bool) {
	if element == "" {
		return out, 0, false
	}

	if element == "now" {
		return time.Now(), DateParsedFromLayout, true
	}

	if DecRegexp.MatchString(element) {
		value, _ := strconv.ParseUint(element, 10, 64)

		if hint == DateLikeHintUnixMilliseconds {
			return fromUnixMilliseconds(value), DateParsedFromTimestamp, true
		}

		if hint == DateLikeHintUnixSeconds {
			return fromUnixSeconds(value), DateParsedFromTimestamp, true
		}

		// If the value is lower than this Unix seconds timestamp representing 3000-01-01, we assume it's a Unix seconds value
		if value <= 32503683661 {
			return fromUnixSeconds(value), DateParsedFromTimestamp, true
		}

		// In all other cases, we assume it's a Unix milliseconds
		return fromUnixMilliseconds(value), DateParsedFromTimestamp, true
	}

	// Try all layouts we support
	return fromLayouts(element)
}

func fromLayouts(element string) (out time.Time, parsedFrom DateParsedFrom, ok bool) {
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, element)
		if err == nil {
			return parsed, DateParsedFromLayout, true
		}
	}

	for _, layout := range localLayouts {
		parsed, err := time.Parse(layout, element)
		if err == nil {
			return adjustBackToLocal(parsed), DateParsedFromLayout, true
		}
	}

	return
}

func fromUnixSeconds(value uint64) time.Time {
	return time.Unix(int64(value), 0).UTC()
}

func fromUnixMilliseconds(value uint64) time.Time {
	ns := (int64(value) % 1000) * int64(time.Millisecond)

	return time.Unix(int64(value)/1000, ns).UTC()
}

func adjustBackToLocal(in time.Time) time.Time {
	return in.Add(-1 * time.Duration(int64(localOffset)) * time.Second)
}

var layouts = []string{
	// Sorted from most probably to less probably
	time.RFC3339,
	time.RFC3339Nano,
	time.UnixDate,
	time.RFC850,
	time.RubyDate,
	time.RFC1123,
	time.RFC1123Z,
	time.RFC822,
	time.RFC822Z,
	time.ANSIC,
	time.Stamp,
	time.StampMilli,
	time.StampMicro,
	time.StampNano,
}

var localLayouts = []string{
	// Seen on some websites
	"Jan-02-2006 15:04:05 PM",
}
