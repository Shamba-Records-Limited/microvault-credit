(function () {
	"use strict";

	const KEY = "admin-sidebar-collapsed";
	const sidebar = document.getElementById("sidebar");
	const toggle = document.getElementById("sidebar-toggle");
	if (!sidebar || !toggle) return;

	const apply = (collapsed) => {
		sidebar.dataset.collapsed = collapsed ? "true" : "false";
		toggle.setAttribute("aria-expanded", String(!collapsed));
		toggle.setAttribute("aria-label", collapsed ? "Expand sidebar" : "Collapse sidebar");
	};

	apply(localStorage.getItem(KEY) === "true");

	toggle.addEventListener("click", () => {
		const collapsed = sidebar.dataset.collapsed !== "true";
		localStorage.setItem(KEY, String(collapsed));
		apply(collapsed);
	});
})();
