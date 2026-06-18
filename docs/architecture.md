# Music Context Platform (MCP) — System Architecture v1.0.0

## 1. System Vision & Design Principles
The Music Context Platform is a decentralized, privacy-first data infrastructure designed to aggregate personal music streaming histories, user sentiment metrics, and deep-cut discovery telemetry. 

### Core Tenets:
- **Zero Telemetry Data Egress:** All personal streaming datasets, database schemas, and data pipelines must remain completely enclosed within the local private network (LAN).
- **Decoupled Compute and Storage:** Compute-heavy LLM inference is treated as an isolated worker node, separate from data aggregation and storage.
- **Spec-Driven Evolution:** All system interfaces, schemas, and routing mechanisms must conform to deterministic markdown specs prior to code implementation.

---

## 2. Infrastructure & Network Topology
The platform crosses multiple hardware nodes on the local network (`192.168.68.0/24`), dividing responsibilities between development, processing, storage, and heavy inference.

### Node Mapping:
1. **Local Development Node (MacBook Pro):**
   - **Role:** Runs the local development workspace, code compilation, and manual CLI utilities.
   - **MCP Client Host:** Runs the target user interface applications (e.g., Claude Desktop, Cursor IDE) that act as the MCP Client.
2. **Worker Compute Node (Windows Desktop — 192.168.68.99):**
   - **Role:** Dedicated hardware-acceleration layer running an open-weights LLM runner daemon (Ollama).
   - **Hardware Env:** AMD Radeon RX 7900 XT (20GB dedicated VRAM).
   - **Model Layer:** `deepseek-r1:14b` (Distilled Qwen, optimized for deep system reasoning and local SQL generation).
3. **Persistent Storage Node (unRAID Server — Target Phase):**
   - **Role:** Future long-term destination for the data layer, migrating from local development SQLite to a persistent network PostgreSQL container.

---

## 3. Core Data & Execution Flows

### Data Ingestion Loop (Idempotent)
1. The user drops a localized data dump (e.g., YouTube Music Takeout CSV) into the file system.
2. The `ingest-cli` binary parses the data using stream processing to minimize memory footprints.
3. Every record passes through a normalization engine to generate a cryptographic SHA-256 natural key.
4. Data is committed via an upsert pattern (`ON CONFLICT DO NOTHING`) to guarantee that duplicate processing loops do not cause database drift or bloating.

### Inference & Tool-Calling Loop (MCP)
1. The user asks the MCP Client application on the MacBook Pro a contextual question (e.g., *"Find all technical death metal bands in my historical library"*).
2. The MCP Client pipes the instruction to the background `mcp-server` binary via standard I/O (stdin/stdout) using JSON-RPC protocol.
3. The `mcp-server` translates the request into an internal SQL block against the local database asset.
4. The structured text payload is formatted as clean Markdown and shipped out across the local LAN to the Worker Compute Node (`192.168.68.99:11434`) where DeepSeek-R1 processes the context block.
5. The final recommendation is passed back down to the user's terminal UI.

---

## 4. Security & Isolation Boundaries
- **Inbound Firewall Access:** The Worker Compute Node limits incoming TCP traffic on port `11434` strictly to the `Private` network profile firewall rule.
- **Local Inference Isolation:** The local Ollama daemon has explicit cloud telemetry overrides disabled (`Ollama Cloud` toggled off), preventing runtime processing steps from escaping to external web proxies or vendor infrastructure.
