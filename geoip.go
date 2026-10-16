package GeoIP

import (
	"embed"
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
)

//go:embed ccc.csv ipinfo_lite.csv hosting.csv
var csvFiles embed.FS

var (
	initializeMu   sync.Mutex
	activeDatabase atomic.Pointer[database]
)

var (
	ErrInvalidIPv4    = errors.New("geoip: not a valid IPv4 address")
	ErrNonPublicIPv4  = errors.New("geoip: not a public IPv4 address")
	ErrNotFound       = errors.New("geoip: IPv4 address not found in the database")
	ErrNotInitialized = errors.New("geoip: database is not initialized")
)

type database struct {
	countries [26 * 26]country
	ranges    []addressRange
	buckets   [1 << 16]indexBucket
}

// IPv4Info contains the country code and hosting-list membership for an address.
type IPv4Info struct {
	CountryCode string
	IsHosting   bool
	country     *country
}

// Continent returns the continent from ccc.csv, or "" for a zero-value result.
func (info IPv4Info) Continent() string {
	if info.country == nil {
		return ""
	}
	return info.country.continent
}

// CountryName returns the country name from ccc.csv, or "" for a zero-value result.
func (info IPv4Info) CountryName() string {
	if info.country == nil {
		return ""
	}
	return info.country.name
}

type country struct {
	code      string
	name      string
	continent string
}

// Initialize validates the embedded ccc.csv, ipinfo_lite.csv, and hosting.csv,
// then builds the package-level in-memory index. It needs no external files.
// Successful initialization is reused, and concurrent calls are safe.
// On failure it returns an error identifying the invalid file or record.
func Initialize() error {
	initializeMu.Lock()
	defer initializeMu.Unlock()
	if activeDatabase.Load() != nil {
		return nil
	}
	database := new(database)
	if err := readCountries(csvFiles, "ccc.csv", &database.countries); err != nil {
		return err
	}
	geography, err := readRanges(csvFiles, "ipinfo_lite.csv", &database.countries)
	if err != nil {
		return err
	}
	hosting, err := readRanges(csvFiles, "hosting.csv", nil)
	if err != nil {
		return err
	}
	database.ranges = combineRanges(geography, hosting)
	database.buildBuckets()
	activeDatabase.Store(database)
	return nil
}

// GetIpv4Info returns information for a public, native IPv4 address.
// IPv6 (including IPv4-mapped IPv6), CIDRs, ports, zones, and malformed input
// return ErrInvalidIPv4. Special-use addresses return ErrNonPublicIPv4, and
// public addresses absent from ipinfo_lite.csv return ErrNotFound.
// Calls before successful initialization return ErrNotInitialized.
func GetIpv4Info(ip string) (IPv4Info, error) {
	database := activeDatabase.Load()
	if database == nil {
		return IPv4Info{}, ErrNotInitialized
	}
	address, err := netip.ParseAddr(ip)
	if err != nil || !address.Is4() {
		return IPv4Info{}, ErrInvalidIPv4
	}
	value := ipv4Number(address)
	bucket := &database.buckets[value>>16]
	if bucket.policy == policyNonPublic || (bucket.policy == policyMixed && !publicRange(value, value)) {
		return IPv4Info{}, ErrNonPublicIPv4
	}
	first, last := bucket.first, bucket.last
	for first < last {
		middle := first + (last-first)/2
		if database.ranges[middle].end < value {
			first = middle + 1
		} else {
			last = middle
		}
	}
	if first == bucket.last || database.ranges[first].start > value {
		return IPv4Info{}, ErrNotFound
	}
	entry := &database.ranges[first]
	metadata := &database.countries[entry.countryID]
	return IPv4Info{CountryCode: metadata.code, IsHosting: entry.isHosting, country: metadata}, nil
}
