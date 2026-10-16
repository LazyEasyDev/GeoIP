package geoip

import (
	"cmp"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type addressRange struct {
	start     uint32
	end       uint32
	countryID uint16
	isHosting bool
}

var nonPublicRanges = [...]addressRange{
	{start: 0x00000000, end: 0x00ffffff},
	{start: 0x0a000000, end: 0x0affffff},
	{start: 0x64400000, end: 0x647fffff},
	{start: 0x7f000000, end: 0x7fffffff},
	{start: 0xa9fe0000, end: 0xa9feffff},
	{start: 0xac100000, end: 0xac1fffff},
	{start: 0xc0000000, end: 0xc0000008},
	{start: 0xc000000b, end: 0xc00000ff},
	{start: 0xc0000200, end: 0xc00002ff},
	{start: 0xc0586300, end: 0xc05863ff},
	{start: 0xc0a80000, end: 0xc0a8ffff},
	{start: 0xc6120000, end: 0xc613ffff},
	{start: 0xc6336400, end: 0xc63364ff},
	{start: 0xcb007100, end: 0xcb0071ff},
	{start: 0xe0000000, end: 0xffffffff},
}

func readRecords(source fs.FS, path string, fields int, consume func([]string, int) error) error {
	file, err := source.Open(path)
	if err != nil {
		return fmt.Errorf("geoip: open %s: %w", path, err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = fields
	reader.ReuseRecord = true
	rows := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("geoip: %s: %w", path, err)
		}
		line, _ := reader.FieldPos(0)
		if err := consume(record, line); err != nil {
			return fmt.Errorf("geoip: %s:%d: %w", path, line, err)
		}
		rows++
	}
	if rows == 0 {
		return fmt.Errorf("geoip: %s: empty CSV", path)
	}
	return nil
}

func readCountries(source fs.FS, path string, countries *[26 * 26]country) error {
	return readRecords(source, path, 3, func(record []string, _ int) error {
		switch record[0] {
		case "Africa", "Antarctica", "Asia", "Europe", "North America", "Oceania", "South America":
		default:
			return fmt.Errorf("invalid continent %q", record[0])
		}
		countryID, valid := countryIndex(record[1])
		if !valid {
			return fmt.Errorf("invalid country code %q", record[1])
		}
		if countries[countryID].code != "" {
			return fmt.Errorf("duplicate country code %q", record[1])
		}
		name := record[2]
		if name == "" || strings.TrimSpace(name) != name || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return fmt.Errorf("invalid country name %q", name)
		}
		countries[countryID] = country{code: record[1], name: name, continent: record[0]}
		return nil
	})
}

func countryIndex(code string) (uint16, bool) {
	if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return 0, false
	}
	return uint16(code[0]-'A')*26 + uint16(code[1]-'A'), true
}

func readRanges(source fs.FS, path string, countries *[26 * 26]country) ([]addressRange, error) {
	ranges := make([]addressRange, 0)
	err := readRecords(source, path, 2, func(record []string, _ int) error {
		entry, err := parseRange(record[0])
		if err != nil {
			return err
		}
		if !publicRange(entry.start, entry.end) {
			return fmt.Errorf("range %q contains non-public IPv4 addresses", record[0])
		}
		if countries == nil {
			if record[1] != "1" {
				return fmt.Errorf("invalid hosting flag %q: expected 1", record[1])
			}
		} else {
			countryID, valid := countryIndex(record[1])
			if !valid || countries[countryID].code == "" {
				return fmt.Errorf("unknown country code %q", record[1])
			}
			entry.countryID = countryID
		}
		ranges = append(ranges, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(ranges, func(left, right addressRange) int {
		return cmp.Compare(left.start, right.start)
	})
	for position := 1; position < len(ranges); position++ {
		previous, current := ranges[position-1], ranges[position]
		if previous.end >= current.start {
			return nil, fmt.Errorf("geoip: %s: overlapping or duplicate ranges %s-%s and %s-%s", path,
				ipv4Address(previous.start), ipv4Address(previous.end), ipv4Address(current.start), ipv4Address(current.end))
		}
	}
	return ranges, nil
}

func parseRange(text string) (addressRange, error) {
	if strings.Contains(text, "/") {
		prefix, err := netip.ParsePrefix(text)
		if err != nil || !prefix.Addr().Is4() {
			return addressRange{}, fmt.Errorf("invalid IPv4 CIDR %q", text)
		}
		if prefix != prefix.Masked() {
			return addressRange{}, fmt.Errorf("non-canonical IPv4 CIDR %q", text)
		}
		start := ipv4Number(prefix.Addr())
		return addressRange{start: start, end: start | (uint32(0xffffffff) >> prefix.Bits())}, nil
	}
	address, err := netip.ParseAddr(text)
	if err != nil || !address.Is4() {
		return addressRange{}, fmt.Errorf("invalid IPv4 address %q", text)
	}
	value := ipv4Number(address)
	return addressRange{start: value, end: value}, nil
}

func publicRange(start, end uint32) bool {
	return rangePolicy(start, end) == policyPublic
}

func rangePolicy(start, end uint32) uint8 {
	position := sort.Search(len(nonPublicRanges), func(position int) bool {
		return nonPublicRanges[position].end >= start
	})
	if position == len(nonPublicRanges) || nonPublicRanges[position].start > end {
		return policyPublic
	}
	if nonPublicRanges[position].start <= start && nonPublicRanges[position].end >= end {
		return policyNonPublic
	}
	return policyMixed
}

func ipv4Number(address netip.Addr) uint32 {
	bytes := address.As4()
	return binary.BigEndian.Uint32(bytes[:])
}

func ipv4Address(value uint32) netip.Addr {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], value)
	return netip.AddrFrom4(bytes)
}
