// Signs the server's SEP-10 style challenge with Freighter and posts it back.
(function () {
	"use strict";

	const button = document.getElementById("signin");
	const errorEl = document.getElementById("signin-error");
	if (!button) return;

	const fail = (message) => {
		errorEl.textContent = message;
		button.disabled = false;
		button.textContent = "Sign in with Freighter";
	};

	// Freighter has changed this return shape across versions; accept both.
	const unwrapSigned = (result) => {
		if (typeof result === "string") return result;
		if (result && typeof result.signedTxXdr === "string") return result.signedTxXdr;
		if (result && typeof result.signedXDR === "string") return result.signedXDR;
		return null;
	};

	const unwrapAddress = (result) => {
		if (typeof result === "string") return result;
		if (result && typeof result.address === "string") return result.address;
		if (result && typeof result.publicKey === "string") return result.publicKey;
		return null;
	};

	button.addEventListener("click", async () => {
		errorEl.textContent = "";
		button.disabled = true;
		button.textContent = "Waiting for wallet…";

		const api = window.freighterApi;
		if (!api) {
			fail("Freighter API failed to load. Reload the page.");
			return;
		}

		try {
			const access = await api.requestAccess();
			const address = unwrapAddress(access);
			if (!address) {
				fail(
					access && access.error
						? access.error
						: "Freighter was not detected. Install the extension and reload.",
				);
				return;
			}

			const challengeRes = await fetch("/login/challenge", { method: "POST" });
			if (!challengeRes.ok) {
				fail("Could not obtain a challenge from the server.");
				return;
			}
			const challenge = await challengeRes.json();

			const signResult = await api.signTransaction(challenge.transaction, {
				networkPassphrase: button.dataset.network,
				address: address,
			});
			const signed = unwrapSigned(signResult);
			if (!signed) {
				fail(
					signResult && signResult.error
						? signResult.error
						: "Freighter did not return a signed transaction.",
				);
				return;
			}

			const verifyRes = await fetch("/login/verify", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ challenge_id: challenge.id, signed_transaction: signed }),
			});

			if (verifyRes.ok) {
				window.location.assign("/");
				return;
			}
			const body = await verifyRes.json().catch(() => ({}));
			fail(body.error || "Verification failed.");
		} catch (err) {
			fail(err && err.message ? err.message : "Signing was cancelled.");
		}
	});
})();
