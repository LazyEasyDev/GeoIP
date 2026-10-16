# Hosting Process

[hosting_process.csv](hosting_process.csv) is the working dataset; [hosting.csv](../hosting.csv) is an optional generated export with no header: `ipv4_cidr,1`. A lookup miss means non-hosting (`-1`).

## 1. Sources to Check

Use this order, reading each line left to right:

- [X4BNet](https://github.com/X4BNet/lists_vpn).
- [AWS](https://ip-ranges.amazonaws.com/ip-ranges.json), [Google Cloud](https://www.gstatic.com/ipranges/cloud.json), [Azure Public](https://www.microsoft.com/en-us/download/details.aspx?id=56519) (`AzureCloud` only), [Oracle](https://docs.oracle.com/iaas/tools/public_ip_ranges.json).
- [bad-asn-list](https://github.com/brianhama/bad-asn-list) joined to an ASN-bearing [IPinfo Lite](https://ipinfo.io/lite) snapshot; the product geography CSV has no ASN.
- [DigitalOcean](https://digitalocean.com/geo/google.csv), [Linode / Akamai](https://geoip.linode.com/), [Vultr](https://geofeed.constant.com/?json), [Scaleway](https://www.scaleway.com/en/docs/account/support/scaleway-network-information.md) (IPv4 ranges only).
- [GitHub](https://api.github.com/meta) (service CIDR arrays only), [Cloudflare](https://www.cloudflare.com/ips-v4), [Fastly](https://api.fastly.com/public-ip-list).
- Explicit ISP classifications last, when available; unknown labels are not negative evidence.

Use [IANA](https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry-1.csv) for public-address filtering, not hosting evidence. Optional IPinfo Core verification uses `is_hosting`, not ASN, VPN, or proxy flags.

## 2. Order and Line Merging

- Use the working dataset as the base; finish applying one source's evidence before starting the next. Apply explicit ISP negatives after all positive feeds.
- Sort by numeric IPv4 start; require canonical, nonoverlapping CIDRs. An individual IP contributes only `/32`.
- Only when the user explicitly asks the AI to split an existing CIDR into smaller CIDRs for further checks, subdivide it with exactly the same coverage and gaps. Reuse any recorded sample only in its containing child, keeping that child's fixed label; mark all other children `x_?`, where `x` is the inherited parent status (e.g. `1_?` or `-1_?`), not a verified result. Never initiate this refinement automatically during source imports.
- For explicitly authorized API checks, freeze one random, previously unqueried IP per pending child. A valid matching saved Core response is the golden rule: replace the checked row's entire old status (including `0`) with fixed `1` (true), `-1` (false), or `0` (missing/null hosting), removing `_?`. Errors or missing/invalid responses leave it pending. Persist samples and request intents before calls; save responses immediately, never resubmit an attempted IP, and use at most 500 IPs/request. Sampling does not verify every address.
- Never coalesce rows in the working dataset, even after verification or when adjacent fixed statuses match. Preserve check boundaries and retain `-1`, `0`, and `x_?` records there.
- Only when generating the export, filter a copy to fixed `1` rows, excluding all `-1`, `0`, and `x_?` records. Then combine only touching hosting intervals with `end + 1 == next_start` and emit the minimal exact CIDR cover. Never fill gaps or enlarge coverage. Write the result to `hosting.csv` without modifying `hosting_process.csv`.

## 3. New Source Merging

Before merging any new source, require that all working `x_?` rows have been resolved to fixed `-1`, `1`, or `0` by saved API results; otherwise stop without changing the working file. Normalize the whole source first. Exclude non-public/special-purpose space and multicast from both base and source; apply IANA's most-specific rule, retaining globally reachable exceptions. Deduplicate and split source overlaps: `1` intersecting explicit `-1` becomes conflict `0`; unknown/missing values supply no evidence. Keep temporary negatives until this source's merge finishes.

Split at every base/source boundary. If an incoming CIDR is larger than overlapping existing rows, split the incoming range into exact canonical pieces around those rows. Preserve existing row boundaries and keep recorded samples only in their containing pieces. This boundary alignment does not create `x_?` labels or initiate further checks. Apply the table per exact piece; source merging means evidence updates, never coalescing working rows, even when labels match:

| Incoming evidence | Existing `1` | Existing `-1` | Existing `0` | No row |
| --- | --- | --- | --- | --- |
| Hosting `1` | Keep `1` | Conflict `0` | Keep `0` | Add `1` |
| Explicit non-hosting `-1` | Conflict `0` | Keep `-1` | Keep `0` | Omit |
| Explicit conflict `0` | Conflict `0` | Conflict `0` | Keep `0` | Add `0` |
| Unknown or missing | Unchanged | Unchanged | Keep `0` | Omit |

Opposite explicit labels (`1` and `-1`) create conflict `0` only on their overlap, in either direction. Preserve unaffected base portions. Every existing `0` stays `0` during new-source merging, whether it originated from conflict or unknown API information; neither CIDR specificity nor the last source wins. This restriction does not apply to explicitly authorized API verification above. A missing row is never explicit negative evidence. Record input versions and merge order; source corrections/removals require replay, not another ordinary addition.
