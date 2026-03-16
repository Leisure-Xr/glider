#!/usr/bin/env python3
"""
Generate a glider proxy-pool config from a v2rayN-style node list.

Input supported (one per line):
  - ss://... (SIP002, optional plugin=obfs-local)
  - vmess://... (v2rayN base64(JSON), common tcp/ws + optional tls)
  - trojan://... (standard trojan URI, tcp/ws + tls)
  - vless://... (standard vless URI, tcp/ws + optional tls)
  - ssr://... (ShadowsocksR base64 encoded)

Output:
  - glider.conf compatible key=value config with forward=... lines.
"""

from __future__ import annotations

import argparse
import base64
import json
import sys
from dataclasses import dataclass
from typing import Optional
from urllib.parse import parse_qs, quote, unquote, urlparse


def _b64_decode(s: str) -> str:
    s = s.strip()
    s += "=" * (-len(s) % 4)
    return base64.urlsafe_b64decode(s.encode("utf-8")).decode("utf-8", errors="strict")


def _quote_userinfo(s: str) -> str:
    # Keep only unreserved characters unescaped to avoid breaking URL parsing.
    return quote(s, safe="-._~")


@dataclass(frozen=True)
class SsNode:
    host: str
    port: int
    method: str
    password: str
    plugin: Optional[str] = None
    name: Optional[str] = None


@dataclass(frozen=True)
class TrojanNode:
    host: str
    port: int
    password: str
    sni: str
    skip_verify: bool
    net: str  # tcp / ws
    ws_host: str
    ws_path: str
    name: Optional[str] = None


@dataclass(frozen=True)
class VlessNode:
    host: str
    port: int
    uuid: str
    encryption: str
    net: str  # tcp / ws
    tls: str
    sni: str
    skip_verify: bool
    ws_host: str
    ws_path: str
    name: Optional[str] = None


@dataclass(frozen=True)
class SsrNode:
    host: str
    port: int
    method: str
    password: str
    protocol: str
    protocol_param: str
    obfs: str
    obfs_param: str
    name: Optional[str] = None


@dataclass(frozen=True)
class VmessNode:
    host: str
    port: int
    uuid: str
    alter_id: int
    security: str
    net: str
    tls: str
    ws_host: str
    ws_path: str
    sni: str
    insecure: bool
    name: Optional[str] = None


def _parse_ss(url: str) -> SsNode:
    u = urlparse(url)
    if u.scheme.lower() != "ss":
        raise ValueError("not ss://")

    host = u.hostname
    port = u.port
    if not host or not port:
        raise ValueError("missing host/port")

    if not u.username:
        raise ValueError("missing ss userinfo")

    decoded = _b64_decode(u.username)
    # Commonly "method:password"
    if ":" not in decoded:
        raise ValueError("invalid ss userinfo decoded (expected method:password)")
    method, password = decoded.split(":", 1)

    qs = parse_qs(u.query)
    plugin = qs.get("plugin", [None])[0]
    name = unquote(u.fragment) if u.fragment else None

    return SsNode(host=host, port=port, method=method, password=password, plugin=plugin, name=name)


def _parse_vmess(url: str) -> VmessNode:
    # v2rayN commonly uses "vmess://<base64(json)>"
    if not url.lower().startswith("vmess://"):
        raise ValueError("not vmess://")
    payload = url[len("vmess://") :].strip()
    raw = _b64_decode(payload)
    obj = json.loads(raw)

    host = str(obj.get("add") or "").strip()
    port = int(str(obj.get("port") or "0").strip() or "0")
    uuid = str(obj.get("id") or "").strip()
    alter_id = int(str(obj.get("aid") or "0").strip() or "0")
    security = str(obj.get("scy") or "").strip().lower()
    net = str(obj.get("net") or "tcp").strip().lower()
    tls = str(obj.get("tls") or "").strip().lower()

    ws_host = str(obj.get("host") or "").strip()
    ws_path = str(obj.get("path") or "").strip()
    sni = str(obj.get("sni") or "").strip()
    insecure = str(obj.get("insecure") or "").strip().lower() in ("1", "true", "yes")

    name = obj.get("ps")
    if isinstance(name, str):
        name = name.strip() or None
    else:
        name = None

    if not host or not port or not uuid:
        raise ValueError("invalid vmess json (need add/port/id)")

    return VmessNode(
        host=host,
        port=port,
        uuid=uuid,
        alter_id=alter_id,
        security=security,
        net=net,
        tls=tls,
        ws_host=ws_host,
        ws_path=ws_path,
        sni=sni,
        insecure=insecure,
        name=name,
    )


def _parse_trojan(url: str) -> TrojanNode:
    u = urlparse(url)
    if u.scheme.lower() not in ("trojan",):
        raise ValueError("not trojan://")
    host = u.hostname
    port = u.port or 443
    if not host:
        raise ValueError("missing host")
    password = unquote(u.username or "")
    if not password:
        raise ValueError("missing trojan password")
    qs = parse_qs(u.query)
    sni = (qs.get("sni") or qs.get("peer") or [host])[0]
    skip_verify = (qs.get("allowInsecure") or qs.get("skipVerify") or ["0"])[0] in ("1", "true")
    net = (qs.get("type") or ["tcp"])[0].lower()
    ws_host = (qs.get("host") or [""])[0]
    ws_path = (qs.get("path") or [""])[0]
    name = unquote(u.fragment) if u.fragment else None
    return TrojanNode(host=host, port=port, password=password, sni=sni,
                      skip_verify=skip_verify, net=net, ws_host=ws_host,
                      ws_path=ws_path, name=name)


def _parse_vless(url: str) -> VlessNode:
    u = urlparse(url)
    if u.scheme.lower() != "vless":
        raise ValueError("not vless://")
    host = u.hostname
    port = u.port
    if not host or not port:
        raise ValueError("missing host/port")
    uuid = unquote(u.username or "")
    if not uuid:
        raise ValueError("missing vless uuid")
    qs = parse_qs(u.query)
    encryption = (qs.get("encryption") or ["none"])[0]
    net = (qs.get("type") or ["tcp"])[0].lower()
    tls = (qs.get("security") or ["none"])[0].lower()
    sni = (qs.get("sni") or [host])[0]
    skip_verify = (qs.get("allowInsecure") or ["0"])[0] in ("1", "true")
    ws_host = (qs.get("host") or [""])[0]
    ws_path = (qs.get("path") or [""])[0]
    name = unquote(u.fragment) if u.fragment else None
    return VlessNode(host=host, port=port, uuid=uuid, encryption=encryption,
                     net=net, tls=tls, sni=sni, skip_verify=skip_verify,
                     ws_host=ws_host, ws_path=ws_path, name=name)


def _parse_ssr(url: str) -> SsrNode:
    if not url.lower().startswith("ssr://"):
        raise ValueError("not ssr://")
    payload = url[len("ssr://"):].strip()
    decoded = _b64_decode(payload)
    # format: host:port:protocol:method:obfs:b64pass/?params
    main_part, _, param_part = decoded.partition("/?")
    parts = main_part.split(":")
    if len(parts) < 6:
        raise ValueError("invalid ssr format")
    host = parts[0]
    port = int(parts[1])
    protocol = parts[2]
    method = parts[3]
    obfs = parts[4]
    password = _b64_decode(parts[5])
    qs = parse_qs(param_part)
    obfs_param = _b64_decode((qs.get("obfsparam") or [""])[0]) if qs.get("obfsparam") else ""
    proto_param = _b64_decode((qs.get("protoparam") or [""])[0]) if qs.get("protoparam") else ""
    remarks = _b64_decode((qs.get("remarks") or [""])[0]) if qs.get("remarks") else None
    return SsrNode(host=host, port=port, method=method, password=password,
                   protocol=protocol, protocol_param=proto_param,
                   obfs=obfs, obfs_param=obfs_param, name=remarks)


def _ss_to_glider_forward(n: SsNode) -> str:
    # Base SS dialer.
    ss_url = f"ss://{_quote_userinfo(n.method)}:{_quote_userinfo(n.password)}@{n.host}:{n.port}"

    if not n.plugin:
        return f"forward={ss_url}"

    # plugin format: "obfs-local;obfs=http;obfs-host=example.com[;obfs-uri=/foo]"
    plugin = n.plugin
    parts = plugin.split(";")
    plugin_name = parts[0].strip().lower()
    opts: dict[str, str] = {}
    for p in parts[1:]:
        if "=" in p:
            k, v = p.split("=", 1)
            opts[k.strip().lower()] = v.strip()

    if plugin_name == "obfs-local":
        obfs_type = (opts.get("obfs") or "http").strip().lower()
        obfs_host = opts.get("obfs-host")
        if not obfs_host:
            raise ValueError("obfs-local plugin missing obfs-host")

        # simple-obfs expects query: type=...&host=...&uri=...
        q = f"type={quote(obfs_type, safe='')}&host={quote(obfs_host, safe='')}"
        obfs_uri = opts.get("obfs-uri") or opts.get("obfs-uri".replace("-", "_"))
        if obfs_uri:
            q += f"&uri={quote(obfs_uri, safe='')}"

        obfs_url = f"simple-obfs://{n.host}:{n.port}?{q}"
        return f"forward={obfs_url},{ss_url}"

    # Unknown plugin: keep a commented hint and fall back to plain SS (may not work without plugin).
    return f"# unsupported ss plugin: {plugin}\nforward={ss_url}"


def _vmess_to_glider_forward(n: VmessNode) -> str:
    # Map v2rayN scy=auto to a concrete value for glider.
    sec = n.security
    if sec in ("", "auto"):
        sec = "aes-128-gcm"

    # glider vmess URL format: vmess://[security:]uuid@host:port[?alterID=num]
    vmess_user = f"{sec}:{n.uuid}" if sec else n.uuid
    vmess_no_addr = f"vmess://{vmess_user}@?alterID={n.alter_id}"

    use_tls = n.tls in ("tls", "1", "true", "yes")
    if n.net == "tcp":
        if use_tls:
            q = f"serverName={quote((n.sni or n.host), safe='')}"
            if n.insecure:
                q += "&skipVerify=true"
            return f"forward=tls://{n.host}:{n.port}?{q},{vmess_no_addr}"
        return f"forward=vmess://{vmess_user}@{n.host}:{n.port}?alterID={n.alter_id}"

    if n.net == "ws":
        path = n.ws_path or "/"
        if not path.startswith("/"):
            path = "/" + path
        host_hdr = n.ws_host or n.host

        if use_tls:
            q = f"serverName={quote((n.sni or n.host), safe='')}"
            if n.insecure:
                q += "&skipVerify=true"
            ws = f"ws://@{path}?host={quote(host_hdr, safe='')}"
            return f"forward=tls://{n.host}:{n.port}?{q},{ws},{vmess_no_addr}"

        ws = f"ws://{n.host}:{n.port}{path}?host={quote(host_hdr, safe='')}"
        return f"forward={ws},{vmess_no_addr}"

    return (
        f"# unsupported vmess transport: net={n.net} tls={n.tls}\n"
        f"forward=vmess://{vmess_user}@{n.host}:{n.port}?alterID={n.alter_id}"
    )


def _trojan_to_glider_forward(n: TrojanNode) -> str:
    # glider trojan: trojan://pass@host:port[?serverName=X][&skipVerify=true]
    # trojan has built-in TLS; for ws transport, chain tls+ws+trojan
    q = f"serverName={quote(n.sni, safe='')}"
    if n.skip_verify:
        q += "&skipVerify=true"

    if n.net == "tcp":
        return f"forward=trojan://{_quote_userinfo(n.password)}@{n.host}:{n.port}?{q}"

    if n.net == "ws":
        path = n.ws_path or "/"
        if not path.startswith("/"):
            path = "/" + path
        host_hdr = n.ws_host or n.host
        tls_url = f"tls://{n.host}:{n.port}?{q}"
        ws_url = f"ws://@{path}?host={quote(host_hdr, safe='')}"
        trojan_url = f"trojan://{_quote_userinfo(n.password)}@"
        return f"forward={tls_url},{ws_url},{trojan_url}"

    return (
        f"# unsupported trojan transport: net={n.net}\n"
        f"forward=trojan://{_quote_userinfo(n.password)}@{n.host}:{n.port}?{q}"
    )


def _vless_to_glider_forward(n: VlessNode) -> str:
    # glider vless: vless://uuid@host:port
    # needs tls/ws chain like vmess
    vless_no_addr = f"vless://{n.uuid}@"
    use_tls = n.tls in ("tls", "xtls", "reality")

    if n.net == "tcp":
        if use_tls:
            q = f"serverName={quote(n.sni, safe='')}"
            if n.skip_verify:
                q += "&skipVerify=true"
            return f"forward=tls://{n.host}:{n.port}?{q},{vless_no_addr}"
        return f"forward=vless://{n.uuid}@{n.host}:{n.port}"

    if n.net == "ws":
        path = n.ws_path or "/"
        if not path.startswith("/"):
            path = "/" + path
        host_hdr = n.ws_host or n.host
        ws_url = f"ws://@{path}?host={quote(host_hdr, safe='')}"
        if use_tls:
            q = f"serverName={quote(n.sni, safe='')}"
            if n.skip_verify:
                q += "&skipVerify=true"
            return f"forward=tls://{n.host}:{n.port}?{q},{ws_url},{vless_no_addr}"
        ws_url = f"ws://{n.host}:{n.port}{path}?host={quote(host_hdr, safe='')}"
        return f"forward={ws_url},{vless_no_addr}"

    return (
        f"# unsupported vless transport: net={n.net} tls={n.tls}\n"
        f"forward=vless://{n.uuid}@{n.host}:{n.port}"
    )


def _ssr_to_glider_forward(n: SsrNode) -> str:
    # glider ssr: ssr://method:pass@host:port?protocol=X&protocol_param=Y&obfs=Z&obfs_param=W
    q_parts = []
    if n.protocol:
        q_parts.append(f"protocol={quote(n.protocol, safe='')}")
    if n.protocol_param:
        q_parts.append(f"protocol_param={quote(n.protocol_param, safe='')}")
    if n.obfs:
        q_parts.append(f"obfs={quote(n.obfs, safe='')}")
    if n.obfs_param:
        q_parts.append(f"obfs_param={quote(n.obfs_param, safe='')}")
    q = "&".join(q_parts)
    userinfo = f"{_quote_userinfo(n.method)}:{_quote_userinfo(n.password)}"
    ssr_url = f"ssr://{userinfo}@{n.host}:{n.port}"
    if q:
        ssr_url += f"?{q}"
    return f"forward={ssr_url}"


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--nodes", default="dev/node.txt", help="Path to node list file")
    ap.add_argument("--out", default="dev/glider.pool.conf", help="Output glider config path")
    ap.add_argument(
        "--verbose",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="Enable verbose logging (health-check and per-connection logs)",
    )
    ap.add_argument("--listen-socks", default="127.0.0.1:10810", help="SOCKS5 listen addr")
    ap.add_argument("--listen-http", default="127.0.0.1:10811", help="HTTP listen addr")
    ap.add_argument("--strategy", default="rr", choices=["rr", "ha", "lha", "dh"])
    ap.add_argument("--dialtimeout", type=int, default=3, help="Dial timeout seconds")
    ap.add_argument("--relaytimeout", type=int, default=0, help="Relay timeout seconds")
    ap.add_argument("--check", default="http://www.gstatic.com/generate_204#expect=204")
    ap.add_argument("--checkinterval", type=int, default=30)
    ap.add_argument("--checktimeout", type=int, default=10)
    ap.add_argument(
        "--checkdisabledonly",
        action=argparse.BooleanOptionalAction,
        default=False,
        help="Only check disabled forwarders",
    )
    ap.add_argument("--maxfailures", type=int, default=3)
    args = ap.parse_args(argv)

    raw_lines: list[str] = []
    with open(args.nodes, "r", encoding="utf-8") as f:
        for line in f:
            s = line.strip()
            if not s or s.startswith("#"):
                continue
            raw_lines.append(s)

    forwards: list[str] = []
    errs: list[str] = []
    for i, s in enumerate(raw_lines, start=1):
        try:
            if s.startswith("ss://"):
                node = _parse_ss(s)
                forwards.append(_ss_to_glider_forward(node))
            elif s.startswith("vmess://"):
                node = _parse_vmess(s)
                forwards.append(_vmess_to_glider_forward(node))
            elif s.startswith("trojan://"):
                node = _parse_trojan(s)
                forwards.append(_trojan_to_glider_forward(node))
            elif s.startswith("vless://"):
                node = _parse_vless(s)
                forwards.append(_vless_to_glider_forward(node))
            elif s.startswith("ssr://"):
                node = _parse_ssr(s)
                forwards.append(_ssr_to_glider_forward(node))
            else:
                errs.append(f"line {i}: unsupported scheme: {s[:20]}...")
        except Exception as e:
            errs.append(f"line {i}: {e}")

    out_lines: list[str] = []
    out_lines.append("# Generated by dev/gen_glider_pool_conf.py")
    out_lines.append(f"verbose={'True' if args.verbose else 'False'}")
    out_lines.append("")
    out_lines.append(f"listen=socks5://{args.listen_socks}")
    out_lines.append(f"listen=http://{args.listen_http}")
    out_lines.append("")
    out_lines.append(f"strategy={args.strategy}")
    out_lines.append(f"dialtimeout={args.dialtimeout}")
    out_lines.append(f"relaytimeout={args.relaytimeout}")
    out_lines.append(f"check={args.check}")
    out_lines.append(f"checkinterval={args.checkinterval}")
    out_lines.append(f"checktimeout={args.checktimeout}")
    out_lines.append(f"checkdisabledonly={'true' if args.checkdisabledonly else 'false'}")
    out_lines.append(f"maxfailures={args.maxfailures}")
    out_lines.append("")
    out_lines.extend(forwards)
    if errs:
        out_lines.append("")
        out_lines.append("# Parse errors:")
        out_lines.extend([f"# {e}" for e in errs])

    with open(args.out, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out_lines))
        f.write("\n")

    print(f"wrote {args.out} with {len(forwards)} forwarders; {len(errs)} errors", file=sys.stderr)
    return 0 if not errs else 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
