"""Compile first: make -C agent/native. Uses synthetic PCAPs, never live capture."""
import ipaddress
import json
import pathlib
import struct
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "agent/native/subgate-ndpi").resolve())


def packet(payload, ipv6=False, udp=False, port=80):
    if udp:
        transport = struct.pack("!HHHH", 42000, port, 8 + len(payload), 0) + payload
    else:
        transport = struct.pack("!HHIIBBHHH", 42000, port, 1, 1, 0x50, 0x18, 65535, 0, 0) + payload
    protocol = 17 if udp else 6
    if ipv6:
        header = struct.pack("!IHBB", 6 << 28, len(transport), protocol, 64)
        header += ipaddress.ip_address("2001:db8::1").packed + ipaddress.ip_address("2001:db8::2").packed
    else:
        header = struct.pack("!BBHHHBBH4s4s", 0x45, 0, 20 + len(transport), 1, 0, 64, protocol, 0,
                             ipaddress.ip_address("192.0.2.1").packed, ipaddress.ip_address("192.0.2.2").packed)
    return bytes(12) + (b"\x86\xdd" if ipv6 else b"\x08\x00") + header + transport


def classify(name, data, expected=None, truncate=False):
    with tempfile.TemporaryDirectory() as directory:
        pcap = pathlib.Path(directory) / "synthetic.pcap"
        header = struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 65535, 1)
        original = len(data)
        if truncate:
            data = data[:-10]
        pcap.write_bytes(header + struct.pack("<IIII", 1800000000, 0, len(data), original) + data)
        result = subprocess.run([binary, "--pcap", str(pcap)], capture_output=True, text=True, timeout=10)
        assert result.stdout.strip(), (name, result.returncode, result.stderr)
        envelope = json.loads(result.stdout.strip().splitlines()[-1])
        assert "payload" not in envelope, name
        if truncate:
            assert result.returncode != 0 and envelope["dropped"] > 0, (name, envelope)
            return
        assert result.returncode == 0 and envelope["flows"], (name, result.stderr, envelope)
        flow = envelope["flows"][0]
        assert set(flow) == {"id", "protocol", "confidence", "version", "timestamp", "bytes", "packets", "dropped", "truncated"}, flow
        if expected:
            assert expected in flow["protocol"], (name, flow)
            # nDPI 4.2 labels BitTorrent signatures DPI_CACHE; preserve the
            # weaker classification instead of escalating its confidence.
            confidence = "guessed" if expected == "BitTorrent" and flow["version"].startswith("4.2.") else "dpi"
            assert flow["confidence"] == confidence, (name, flow)
        else:
            assert "BitTorrent" not in flow["protocol"], (name, flow)
        print(name, flow["protocol"], flow["confidence"])


handshake = b"\x13BitTorrent protocol" + bytes(8) + bytes(range(20)) + b"-SG0001-abcdefghijkl"
assert len(handshake) == 68
classify("torrent IPv4", packet(handshake, port=6881), "BitTorrent")
classify("torrent IPv6", packet(handshake, ipv6=True, port=6881), "BitTorrent")
classify("browser HTTP", packet(b"GET / HTTP/1.1\r\nHost: example.invalid\r\nUser-Agent: Synthetic\r\n\r\n"), "HTTP")
classify("WebRTC STUN", packet(struct.pack("!HHI", 1, 0, 0x2112A442) + bytes(range(12)), udp=True, port=3478))
classify("game UDP", packet(b"\xff\xff\xff\xffTSource Engine Query\x00", udp=True, port=27015))
classify("VPN WireGuard", packet(b"\x01\x00\x00\x00" + bytes(range(144)), udp=True, port=51820))
classify("encrypted unknown", packet(bytes(range(128)), port=443))
classify("truncated packet", packet(handshake), truncate=True)

# The same TCP tuple may be reused for another application; a new SYN must
# reset nDPI state instead of carrying a torrent classification into HTTP.
with tempfile.TemporaryDirectory() as directory:
    pcap = pathlib.Path(directory) / "reused-tuple.pcap"
    syn = bytearray(packet(b"", port=6881))
    syn[14 + 20 + 13] = 2
    syn[14 + 20 + 4:14 + 20 + 8] = struct.pack("!I", 100)
    data = struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 65535, 1)
    http = bytearray(packet(b"GET / HTTP/1.1\r\nHost: example.invalid\r\n\r\n", port=6881))
    http[14 + 20 + 4:14 + 20 + 8] = struct.pack("!I", 101)
    packets = [packet(handshake, port=6881), bytes(syn), bytes(http)]
    for index, captured in enumerate(packets):
        data += struct.pack("<IIII", 1800000000 + index, 0, len(captured), len(captured)) + captured
    pcap.write_bytes(data)
    result = subprocess.run([binary, "--pcap", str(pcap)], capture_output=True, text=True, timeout=10)
    assert result.returncode == 0, result.stderr
    flows = [flow for line in result.stdout.splitlines() for flow in json.loads(line)["flows"]]
    assert len({flow["id"] for flow in flows}) == 2, flows
    assert "BitTorrent" in flows[0]["protocol"] and "HTTP" in flows[-1]["protocol"], flows
    print("TCP tuple reuse resets classifier: PASS")
print("nDPI synthetic corpus: PASS")
