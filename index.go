package GeoIP

const (
	policyNonPublic uint8 = iota
	policyPublic
	policyMixed
)

type indexBucket struct {
	first  uint32
	last   uint32
	policy uint8
}

func combineRanges(geography, hosting []addressRange) []addressRange {
	ranges := make([]addressRange, 0, len(geography)+2*len(hosting))
	hostingPosition := 0
	for _, location := range geography {
		cursor := location.start
		for hostingPosition < len(hosting) && hosting[hostingPosition].end < cursor {
			hostingPosition++
		}
		for hostingPosition < len(hosting) && hosting[hostingPosition].start <= location.end {
			hosted := hosting[hostingPosition]
			if cursor < hosted.start {
				ranges = appendRange(ranges, addressRange{
					start: cursor, end: hosted.start - 1, countryID: location.countryID,
				})
			}
			end := min(location.end, hosted.end)
			ranges = appendRange(ranges, addressRange{
				start: max(cursor, hosted.start), end: end, countryID: location.countryID, isHosting: true,
			})
			cursor = end + 1
			if hosted.end > location.end {
				break
			}
			hostingPosition++
		}
		if cursor <= location.end {
			ranges = appendRange(ranges, addressRange{
				start: cursor, end: location.end, countryID: location.countryID,
			})
		}
	}
	return ranges
}

func appendRange(ranges []addressRange, entry addressRange) []addressRange {
	if len(ranges) != 0 {
		previous := &ranges[len(ranges)-1]
		if previous.end+1 == entry.start && previous.countryID == entry.countryID && previous.isHosting == entry.isHosting {
			previous.end = entry.end
			return ranges
		}
	}
	return append(ranges, entry)
}

func (database *database) buildBuckets() {
	for position := range database.buckets {
		start := uint32(position) << 16
		database.buckets[position].policy = rangePolicy(start, start|0xffff)
	}
	for position, entry := range database.ranges {
		for bucketID := entry.start >> 16; bucketID <= entry.end>>16; bucketID++ {
			bucket := &database.buckets[bucketID]
			if bucket.last == 0 {
				bucket.first = uint32(position)
			}
			bucket.last = uint32(position + 1)
		}
	}
}
