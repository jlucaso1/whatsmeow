import "./wasm_exec.js";

const whatsmeowBridge = {
	displayQRCode: (qrString: string) => {
		console.log("QR CODE:", qrString);
	},

	dialWebSocket: (url: string, goCallbacks: any) => {
		console.log(`JS Bridge: Dialing WebSocket to ${url}`);
		try {
			const ws = new WebSocket(url);
			ws.binaryType = "arraybuffer";

			ws.onopen = () => {
				console.log("JS Bridge: WebSocket connection opened.");
				goCallbacks.onOpen();
			};

			ws.onmessage = (event: MessageEvent) => {
				const data = new Uint8Array(event.data);
				goCallbacks.onMessage(data);
			};

			ws.onerror = (event: Event) => {
				console.error("JS Bridge: WebSocket error:", event);
				goCallbacks.onError(
					new Error("WebSocket error occurred").toString()
				);
			};

			ws.onclose = (event: CloseEvent) => {
				console.log(
					`JS Bridge: WebSocket closed. Code: ${event.code}, Reason: ${event.reason}`
				);
				goCallbacks.onClose(event.code, event.reason);
			};

			return {
				writeMessage: (data: Uint8Array): void => {
					if (ws.readyState === WebSocket.OPEN) {
						ws.send(data);
					} else {
						console.error(
							"JS Bridge: Attempted to write to a closed WebSocket."
						);
						goCallbacks.onError(
							new Error(
								"Attempted to write to a closed WebSocket"
							).toString()
						);
					}
				},
				close: (code: number, reason: string): void => {
					ws.close(code, reason);
				},
			};
		} catch (e: any) {
			console.error("JS Bridge: Failed to create WebSocket:", e);
			goCallbacks.onError(e.toString());
			return null;
		}
	},
};

interface IWhatsMeowStorage {
	getDevice(jid: string): Promise<string | null>;
	putDevice(jid: string, data: string): Promise<void>;

	getSession(address: string): Promise<Uint8Array | null>;
	putSession(address: string, sessionData: Uint8Array): Promise<void>;

	getIdentity(address: string): Promise<Uint8Array | null>;
	putIdentity(address: string, key: Uint8Array): Promise<void>;

	putLIDMapping(lid: string, pn: string): Promise<void>;
	getLIDForPN(pn: string): Promise<string | null>;
	getPNForLID(lid: string): Promise<string | null>;
}

class Storage implements IWhatsMeowStorage {
	private sessions = new Map<string, Uint8Array>();
	private devices = new Map<string, string>();
	private identities = new Map<string, Uint8Array>();
	private lidToPn = new Map<string, string>();
	private pnToLid = new Map<string, string>();
	constructor() {
		console.log("BRIDGE: Using ServerStorage (In-Memory).");
	}
	public async getDevice(jid: string) {
		return this.devices.get(jid) || null;
	}
	public async putDevice(jid: string, data: string) {
		this.devices.set(jid, data);
	}
	public async getSession(address: string) {
		return this.sessions.get(address) || null;
	}
	public async putSession(address: string, sessionData: Uint8Array) {
		this.sessions.set(address, sessionData);
	}
	public async getIdentity(address: string): Promise<Uint8Array | null> {
		return this.identities.get(address) || null;
	}
	public async putIdentity(address: string, key: Uint8Array): Promise<void> {
		this.identities.set(address, key);
	}
	public async putLIDMapping(lid: string, pn: string): Promise<void> {
		this.lidToPn.set(lid, pn);
		this.pnToLid.set(pn, lid);
	}
	public async getLIDForPN(pn: string): Promise<string | null> {
		return this.pnToLid.get(pn) || null;
	}
	public async getPNForLID(lid: string): Promise<string | null> {
		return this.lidToPn.get(lid) || null;
	}
}

(globalThis as any).displayQRCode = whatsmeowBridge.displayQRCode;

(globalThis as any).dialWebSocket = whatsmeowBridge.dialWebSocket;

const storageImplementation = new Storage();

(globalThis as any).whatsmeowStorage = storageImplementation;

// @ts-ignore go wasm
const go = new Go();

const { instance } = await WebAssembly.instantiateStreaming(
	fetch(new URL("./whatsmeow.wasm", import.meta.url)),
	go.importObject
);

go.run(instance);
