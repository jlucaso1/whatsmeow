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
	getPreKey(keyId: number): Promise<string | null>;
	putPreKey(keyId: number, keyData: string): Promise<void>;
	removePreKey(keyId: number): Promise<void>;
	getHighestPreKeyID(): Promise<number>;

	getPushName(jid: string): Promise<string | null>;
	putPushName(jid: string, name: string): Promise<void>;

	getAppStateVersion(name: string): Promise<string | null>; // Returns JSON of {version, hash}
	putAppStateVersion(name: string, data: string): Promise<void>;
}

class Storage implements IWhatsMeowStorage {
	private sessions = new Map<string, Uint8Array>();
	private devices = new Map<string, string>();
	private identities = new Map<string, Uint8Array>();
	private lidToPn = new Map<string, string>();
	private pnToLid = new Map<string, string>();
	private preKeys = new Map<number, string>();
	private pushNames = new Map<string, string>();
	private appStateVersions = new Map<string, string>();
	private highestPreKeyID = 0;

	constructor() {
		console.log("BRIDGE: Using ServerStorage (In-Memory).");
	}
	public getDevice = async (jid: string) => this.devices.get(jid) || null;
	public putDevice = async (jid: string, data: string) => {
		this.devices.set(jid, data);
	};

	public getSession = async (address: string) =>
		this.sessions.get(address) || null;
	public putSession = async (address: string, data: Uint8Array) => {
		this.sessions.set(address, data);
	};
	public deleteSession = async (address: string) => {
		this.sessions.delete(address);
	};
	public getAllSessionsFor = async (phone: string) => {
		const result: Record<string, Uint8Array> = {};
		for (const [key, value] of this.sessions.entries()) {
			if (key.startsWith(phone)) {
				result[key] = value;
			}
		}
		return result;
	};

	public getIdentity = async (address: string) =>
		this.identities.get(address) || null;
	public putIdentity = async (address: string, key: Uint8Array) => {
		this.identities.set(address, key);
	};

	public putLIDMapping = async (lid: string, pn: string) => {
		this.lidToPn.set(lid, pn);
		this.pnToLid.set(pn, lid);
	};
	public getLIDForPN = async (pn: string) => this.pnToLid.get(pn) || null;
	public getPNForLID = async (lid: string) => this.lidToPn.get(lid) || null;

	public getPreKey = async (keyId: number) => this.preKeys.get(keyId) || null;
	public putPreKey = async (keyId: number, keyData: string) => {
		this.preKeys.set(keyId, keyData);
		if (keyId > this.highestPreKeyID) {
			this.highestPreKeyID = keyId;
		}
	};
	public removePreKey = async (keyId: number) => {
		this.preKeys.delete(keyId);
	};
	public getHighestPreKeyID = async (): Promise<number> =>
		this.highestPreKeyID;

	public getPushName = async (jid: string) => this.pushNames.get(jid) || null;
	public putPushName = async (jid: string, name: string) => {
		this.pushNames.set(jid, name);
	};

	public getAppStateVersion = async (name: string) =>
		this.appStateVersions.get(name) || null;
	public putAppStateVersion = async (name: string, data: string) => {
		this.appStateVersions.set(name, data);
	};
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
