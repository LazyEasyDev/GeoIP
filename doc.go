/*
Package geoip provides read-only country and hosting lookups for public IPv4
addresses using three embedded CSV files. It has no external dependencies
and performs no network requests.

Initialize validates these headerless files embedded at build time:

  - ccc.csv: continent, uppercase two-letter country code, country name.
  - ipinfo_lite.csv: canonical IPv4 CIDR or bare IPv4 address, country code.
  - hosting.csv: canonical IPv4 CIDR or bare IPv4 address, the literal 1.

No directory argument or external data files are needed at runtime. Updating
the CSV data requires rebuilding the application.

Initialization rejects empty files, malformed CSV, incorrect field counts,
invalid country metadata, duplicate country codes, unknown country references,
invalid or non-canonical IPv4 ranges, non-public address space, and overlapping
or duplicate ranges within either address file. Input order does not matter.
Bare addresses are treated as /32. Validation checks data consistency, not the
real-world accuracy or freshness of the supplied geolocation classifications.

The resulting database is immutable and supports concurrent lookups without
locks or disk access. Country and hosting boundaries are combined in memory;
a /16 directory limits each lookup to a small binary search. No source files
are rewritten, reordered, or compacted on disk.

A successful result has CountryCode and IsHosting fields, plus Continent and
CountryName methods. IsHosting reports membership in hosting.csv: false means
the address is not listed, not that it has been independently verified as a
non-hosting address. A country record is required even if an address is listed
in hosting.csv. Failed lookups return a zero-value IPv4Info and a sentinel error
that can be checked with errors.Is.

Public-address exclusions follow the IANA IPv4 Special-Purpose Address Registry
(updated 2025-10-09), including the globally reachable 192.0.0.9 and 192.0.0.10
exceptions. Private, shared, loopback, link-local, documentation, benchmarking,
deprecated 6to4 relay, multicast, reserved, and broadcast ranges are rejected.
This is address-space classification, not a live routing or reachability test.

Typical use:

	database, err := geoip.Initialize()
	if err != nil {
		log.Fatal(err)
	}
	info, err := database.GetIpv4Info("8.8.8.8")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(info.CountryCode, info.IsHosting, info.Continent(), info.CountryName())
*/
package geoip
