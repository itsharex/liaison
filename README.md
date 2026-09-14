# <img src="docs/diagrams/liaison-logo.svg" height="40" align="absmiddle" alt="Liaison logo" /> Liaison

> **AI-powered zero-trust access for private applications.**

Reach private services over SSH/SFTP, database, cache, S3 object storage, desktop, Web, and LLM protocols. Work with an AI Agent in your SSH and database sessions, or ask the home Agent about the connectors, devices, and applications you can access. Self-hosted, with outbound connectors and permission-controlled tools.

[![Go](https://github.com/liaisonio/liaison/actions/workflows/go.yml/badge.svg)](https://github.com/liaisonio/liaison/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/liaisonio/liaison)](https://goreportcard.com/report/github.com/liaisonio/liaison)
[![Release](https://img.shields.io/github/v/release/liaisonio/liaison?display_name=tag&sort=semver)](https://github.com/liaisonio/liaison/releases)
[![License](https://img.shields.io/badge/License-AGPLv3-blue.svg?logo=gnu)](LICENSE)

English | [简体中文](./README_zh.md) | [日本語](./README_ja.md) | [한국어](./README_ko.md) | [Español](./README_es.md) | [Français](./README_fr.md) | [Deutsch](./README_de.md)

[Website](https://liaison.cloud) · [Docs](https://liaison.cloud/docs/get-started/introduction) · [Features](#features) · [Install](#install) · [Access protocols](#supported-access-protocols) · [Agent models](#agent-model-providers) · [Product tour](#product-tour)

![Liaison overview with multi-application traffic](docs/assets/readme/overview-dark-v3.png)

## Features

- ✨ **AI in your workflow** — Inspect terminal output, draft commands, and query databases with an Agent tied to your connection. Tool access follows user permissions; operations requiring approval wait for your confirmation.
- 💬 **Ask about your resources** — Find and inspect your connectors, devices, and applications from the home Agent. Resource visibility stays scoped to the signed-in user.
- 🔌 **Outbound-only connectors** — connect private networks without opening inbound ports on them.
- 🔐 **Application access** — publish TCP, HTTP, HTTPS, WebSocket, and SSH services with per-access controls.
- 🖥️ **Browser workspaces** — WebSSH, WebSFTP, WebRDP, WebVNC, MySQL, MariaDB, PostgreSQL, SQL Server, Oracle, ClickHouse, MongoDB, Elasticsearch, OpenSearch, Redis, and Memcached.
- 📁 **Files and objects** — Manage remote files with WebSFTP. Browse buckets, prefixes, and object metadata from S3-compatible services with WebS3 (currently read-only).
- 🤖 **Private model access** — OpenAI-compatible, Anthropic Messages, and Ollama upstreams with scoped API keys, model mappings, usage records, and per-key Token quotas.
- 🔎 **Application discovery** — scan connector devices and register discovered services from the console.
- 👥 **Identity and access management** — organize users and resources, with Casbin-backed authorization.
- 🛡️ **Firewall policies** — restrict TCP and HTTP access by source IP and CIDR.
- 📋 **Logs and audit** — record management actions and supported application sessions in one place.
- 📦 **Self-hosted deployment** — run the complete control plane on your own Linux server.

## Install

Download the self-contained Docker bundle, extract it, and run the installer (Docker 20.10+ with Compose is required):

```bash
wget https://github.com/liaisonio/liaison/releases/download/v1.13.0/liaison-1.13.0-linux-amd64.tar.gz
tar -xzf liaison-1.13.0-linux-amd64.tar.gz
cd liaison-1.13.0-linux-amd64
./install.sh
```

Open `https://<server-address>` after installation. The installer prints the initial sign-in credentials.

## Supported access protocols

Access your existing private services through Liaison.

<table>
  <tr><th align="left">SSH / SFTP</th><td><img src="docs/assets/integrations/ssh.svg" width="40" height="40" alt="SSH / WebSSH" title="SSH / WebSSH" />&nbsp;&nbsp; <img src="docs/assets/integrations/sftp.svg" width="40" height="40" alt="WebSFTP" title="WebSFTP" /></td></tr>
  <tr><th align="left">Desktop</th><td><img src="docs/assets/integrations/windows.svg" width="40" height="40" alt="WebRDP" title="WebRDP" />&nbsp;&nbsp; <img src="docs/assets/integrations/vnc.png" width="40" height="40" alt="WebVNC" title="WebVNC" /></td></tr>
  <tr><th align="left">SQL databases</th><td><img src="docs/assets/integrations/mysql.svg" width="40" height="40" alt="WebMySQL" title="WebMySQL" />&nbsp;&nbsp; <img src="docs/assets/integrations/mariadb.svg" width="40" height="40" alt="WebMariaDB" title="WebMariaDB" />&nbsp;&nbsp; <img src="docs/assets/integrations/postgresql.svg" width="40" height="40" alt="WebPostgreSQL" title="WebPostgreSQL" />&nbsp;&nbsp; <img src="docs/assets/integrations/sqlserver.svg" width="40" height="40" alt="WebSQLServer" title="WebSQLServer" />&nbsp;&nbsp; <img src="docs/assets/integrations/oracle.svg" width="40" height="40" alt="WebOracle" title="WebOracle" />&nbsp;&nbsp; <img src="docs/assets/integrations/clickhouse.svg" width="40" height="40" alt="WebClickHouse" title="WebClickHouse" /></td></tr>
  <tr><th align="left">Data & search</th><td><img src="docs/assets/integrations/mongodb.svg" width="40" height="40" alt="WebMongoDB" title="WebMongoDB" />&nbsp;&nbsp; <img src="docs/assets/integrations/elasticsearch.svg" width="40" height="40" alt="WebElasticsearch" title="WebElasticsearch" />&nbsp;&nbsp; <img src="docs/assets/integrations/opensearch.svg" width="40" height="40" alt="WebOpenSearch" title="WebOpenSearch" /></td></tr>
  <tr><th align="left">Cache</th><td><img src="docs/assets/integrations/redis.svg" width="40" height="40" alt="WebRedis" title="WebRedis" />&nbsp;&nbsp; <img src="docs/assets/integrations/memcached.svg" width="40" height="40" alt="WebMemcached" title="WebMemcached" /></td></tr>
  <tr><th align="left">Storage</th><td><img src="docs/assets/integrations/s3.svg" width="40" height="40" alt="WebS3 (S3-compatible)" title="WebS3 (S3-compatible)" /></td></tr>
  <tr><th align="left">LLM upstream protocols</th><td><img src="docs/assets/integrations/openai.svg" width="40" height="40" alt="OpenAI-compatible" title="OpenAI-compatible" />&nbsp;&nbsp; <img src="docs/assets/integrations/anthropic.svg" width="40" height="40" alt="Anthropic Messages" title="Anthropic Messages" />&nbsp;&nbsp; <img src="docs/assets/integrations/ollama.svg" width="40" height="40" alt="Ollama" title="Ollama" /></td></tr>
  <tr><th align="left">Web / TCP</th><td><img src="docs/assets/integrations/web.svg" width="40" height="40" alt="HTTP / HTTPS / WebSocket" title="HTTP / HTTPS / WebSocket" />&nbsp;&nbsp; <img src="docs/assets/integrations/tcp.svg" width="40" height="40" alt="TCP" title="TCP" /></td></tr>
</table>

LLM upstreams support **OpenAI-compatible**, **Anthropic Messages**, and **Ollama**. Clients use OpenAI-compatible endpoints; Anthropic upstreams also expose native Messages endpoints. Logos identify existing services you can access, not products bundled or deployed by Liaison. Database and desktop logos refer to browser workspaces, not native database/RDP/VNC server listeners.

## Agent model providers

Configure the model behind Liaison’s built-in Agent, independently of service access.

<table>
  <tr>
    <th align="left">Models</th>
    <td><img src="docs/assets/integrations/openai.svg" width="40" height="40" alt="OpenAI" title="OpenAI" />&nbsp;&nbsp; <img src="docs/assets/integrations/anthropic.svg" width="40" height="40" alt="Anthropic / Claude" title="Anthropic / Claude" />&nbsp;&nbsp; <img src="docs/assets/integrations/gemini.svg" width="40" height="40" alt="Google Gemini" title="Google Gemini" />&nbsp;&nbsp; <img src="docs/assets/integrations/deepseek.svg" width="40" height="40" alt="DeepSeek" title="DeepSeek" />&nbsp;&nbsp; <img src="docs/assets/integrations/zhipu.svg" width="40" height="40" alt="Z.ai / GLM" title="Z.ai / GLM" />&nbsp;&nbsp; <img src="docs/assets/integrations/kimi.svg" width="40" height="40" alt="Moonshot / Kimi" title="Moonshot / Kimi" />&nbsp;&nbsp; <img src="docs/assets/integrations/minimax.svg" width="40" height="40" alt="MiniMax" title="MiniMax" />&nbsp;&nbsp; <img src="docs/assets/integrations/mimo.svg" width="40" height="40" alt="Xiaomi MiMo" title="Xiaomi MiMo" /></td>
  </tr>
</table>

Eight provider presets, plus custom OpenAI-compatible services.

## Product tour

### Web SSH

Work in an audited browser terminal with an Agent that can inspect session output and help draft commands.

![Using Web SSH through Liaison](docs/assets/readme/web-ssh-dark.png)

### Private web applications

Access media servers and other internal web applications through Liaison.

![Accessing Jellyfin through Liaison](docs/pages/jellyfin-ss.png)

### Private AI assistants

Keep internal AI tools reachable without exposing the private network.

![Accessing OpenClaw through Liaison](docs/pages/openclaw-ss.png)

### Web MySQL

Browse schemas, run SQL, and ask the session Agent to query and explain results, with approval where required.

![Using MySQL through Liaison](docs/assets/readme/web-mysql-dark.png)

### Web MongoDB

Explore collections and ask the session Agent to run approved queries and explain documents, without a local client.

![Using MongoDB through Liaison](docs/assets/readme/web-mongodb-dark.png)

### Web VNC

Open a private VNC desktop in a managed browser session.

![A VNC desktop opened in Liaison](docs/assets/readme/web-vnc-dark.png)

### Web RDP

Connect to an RDP desktop from the same access workflow.

![An RDP desktop opened in Liaison](docs/assets/readme/web-rdp-dark-v2.png)

## Documentation

- [Docker deployment](deploy/docker/README.md)
- [API reference](docs/swagger/swagger.yaml)
- [Release notes](https://github.com/liaisonio/liaison/releases)
- [Issues](https://github.com/liaisonio/liaison/issues)
- [Discussions](https://github.com/liaisonio/liaison/discussions)

## Contributing

Bug reports, feature proposals, documentation improvements, and pull requests are welcome. Start with [Issues](https://github.com/liaisonio/liaison/issues) or open a [Pull Request](https://github.com/liaisonio/liaison/pulls).

## License

Liaison is licensed under the [GNU Affero General Public License v3.0](LICENSE).
