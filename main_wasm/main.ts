import "./wasm_exec.js";

interface IWhatsMeowStorage {
	getDevice(jid: string): Promise<any | null>;
	putDevice(jid: string, data: any): Promise<void>;
	getSession(address: string): Promise<Uint8Array | null>;
	putSession(address: string, sessionData: Uint8Array): Promise<void>;
}

class Storage implements IWhatsMeowStorage {
	private sessions = new Map<string, Uint8Array>();
	private devices = new Map<string, any>();
	constructor() {
		console.log("BRIDGE: Using ServerStorage (In-Memory).");
	}
	public async getDevice(jid: string) {
		return this.devices.get(jid) || null;
	}
	public async putDevice(jid: string, data: any) {
		this.devices.set(jid, data);
	}
	public async getSession(address: string) {
		return this.sessions.get(address) || null;
	}
	public async putSession(address: string, sessionData: Uint8Array) {
		this.sessions.set(address, sessionData);
	}
}

(globalThis as any).displayQRCode = (qrString: string) => {
	console.log("-----------------------------------------");
	console.log("QR CODE RECEIVED via global callback:");
	console.log(qrString);
	console.log("-----------------------------------------");
};

(globalThis as any).dialWebSocket = (url: string, goCallbacks: any) => {
	console.log(`JS Bridge: Dialing WebSocket to ${url}`);
	try {
		const ws = new WebSocket(url);
		ws.binaryType = "arraybuffer"; // Go expects raw binary data

		ws.onopen = () => {
			console.log("JS Bridge: WebSocket connection opened.");
			goCallbacks.onOpen();
		};

		ws.onmessage = (event: MessageEvent) => {
			// event.data will be an ArrayBuffer. We wrap it in a Uint8Array for Go.
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

		// Return an object of functions that Go can call to interact with the socket
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
};

const storageImplementation = new Storage();

(globalThis as any).whatsmeowStorage = storageImplementation;

// @ts-ignore ok
const go = new Go();

const { instance } = await WebAssembly.instantiateStreaming(
	fetch(new URL("./whatsmeow.wasm", import.meta.url)),
	go.importObject
);

go.run(instance);
