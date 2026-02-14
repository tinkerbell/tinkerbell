// Tinkerbell Web UI Application JavaScript

// Global constants - must match backend values (templates.AllNamespace)
const ALL_NAMESPACE = "All";

// Get base URL from body data attribute (set by server config)
function getBaseURL() {
	return document.body.dataset.baseUrl || '';
}

// HTMX request interceptor - prepend baseURL to all relative paths
// This allows templates to use simple paths like "/hardware" without URL prefix concerns
document.body.addEventListener('htmx:configRequest', function(evt) {
	const baseURL = getBaseURL();
	if (baseURL && evt.detail.path.startsWith('/') && !evt.detail.path.startsWith(baseURL)) {
		evt.detail.path = baseURL + evt.detail.path;
	}
});

// Prefix all internal links with baseURL
// This modifies href attributes directly so browser navigation works correctly
function prefixInternalLinks(root) {
	const baseURL = getBaseURL();
	if (!baseURL) return;
	
	const links = (root || document).querySelectorAll('a[href^="/"]');
	links.forEach(link => {
		const href = link.getAttribute('href');
		if (!href.startsWith(baseURL)) {
			link.setAttribute('href', baseURL + href);
		}
	});
}

// Run on page load
prefixInternalLinks();

// Run after HTMX swaps in new content
document.body.addEventListener('htmx:afterSwap', function(evt) {
	prefixInternalLinks(evt.detail.target);
});

// HTML escape function to prevent XSS attacks when rendering user-controlled content
function escapeHtml(text) {
	const div = document.createElement('div');
	div.textContent = text;
	return div.innerHTML;
}

// Handle clickable table rows with data-href attribute
document.addEventListener('click', (event) => {
	// Don't handle clicks on buttons, links, or interactive elements
	if (event.target.closest('button, a, input, select, textarea')) {
		return;
	}
	
	const row = event.target.closest('.clickable-row');
	if (row && row.dataset.href) {
		// Prepend baseURL to relative paths
		const href = row.dataset.href;
		const baseURL = getBaseURL();
		window.location.href = href.startsWith('/') ? baseURL + href : href;
	}
});

// Fallback copy method for non-secure (HTTP) contexts where navigator.clipboard is unavailable
function fallbackCopyText(text, onSuccess) {
	const textarea = document.createElement('textarea');
	textarea.value = text;
	textarea.style.position = 'fixed';
	textarea.style.left = '-9999px';
	textarea.style.top = '-9999px';
	document.body.appendChild(textarea);
	textarea.focus();
	textarea.select();

	try {
		document.execCommand('copy');
		onSuccess();
	} catch (err) {
		console.error('Fallback copy failed:', err);
	}

	document.body.removeChild(textarea);
}

// Handle copy buttons with data-copy-target attribute
document.addEventListener('click', (event) => {
	const button = event.target.closest('.copy-btn');
	if (button && button.dataset.copyTarget) {
		const element = document.getElementById(button.dataset.copyTarget);
		if (element) {
			const text = element.textContent;
			function showCopied() {
				const originalHTML = button.innerHTML;
				button.innerHTML = '<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>Copied!';
				setTimeout(() => {
					button.innerHTML = originalHTML;
				}, 2000);
			}

			if (navigator.clipboard && window.isSecureContext) {
				navigator.clipboard.writeText(text).then(showCopied).catch(() => {
					fallbackCopyText(text, showCopied);
				});
			} else {
				fallbackCopyText(text, showCopied);
			}
		}
	}
});

// YAML Syntax Highlighting
function highlightYAML(code) {
	// Escape HTML first
	let escaped = code
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;');
	
	// Split into lines and process each
	return escaped.split('\n').map(line => {
		// Comments (lines starting with #)
		if (/^\s*#/.test(line)) {
			return '<span class="yaml-comment">' + line + '</span>';
		}
		
		// Key-value pairs
		return line.replace(
			/^(\s*)([a-zA-Z_][a-zA-Z0-9_-]*)(:)(\s*)(.*)$/,
			(match, indent, key, colon, space, value) => {
				let highlightedValue = value;
				
				// String values in quotes
				if (/^["'].*["']$/.test(value)) {
					highlightedValue = '<span class="yaml-string">' + value + '</span>';
				}
				// Numbers
				else if (/^-?\d+\.?\d*$/.test(value)) {
					highlightedValue = '<span class="yaml-number">' + value + '</span>';
				}
				// Booleans
				else if (/^(true|false|yes|no|on|off)$/i.test(value)) {
					highlightedValue = '<span class="yaml-boolean">' + value + '</span>';
				}
				// Null
				else if (/^(null|~)$/i.test(value)) {
					highlightedValue = '<span class="yaml-null">' + value + '</span>';
				}
				// Anchors and aliases
				else if (/^[&*]/.test(value)) {
					highlightedValue = '<span class="yaml-anchor">' + value + '</span>';
				}
				// Template variables like {{.device_1}}
				else if (/\{\{.*\}\}/.test(value)) {
					highlightedValue = value.replace(/(\{\{[^}]+\}\})/g, '<span class="yaml-template">$1</span>');
				}
				
				return indent + '<span class="yaml-key">' + key + '</span>' + 
					   '<span class="yaml-colon">' + colon + '</span>' + space + highlightedValue;
			}
		)
		// List items (lines starting with -)
		.replace(/^(\s*)(-)(\s)/, '$1<span class="yaml-dash">$2</span>$3');
	}).join('\n');
}

// Apply YAML highlighting to all yaml-code elements
document.addEventListener('DOMContentLoaded', () => {
	document.querySelectorAll('.yaml-code').forEach(el => {
		el.innerHTML = highlightYAML(el.textContent);
	});
});

// Also apply highlighting after HTMX swaps (for dynamic content)
document.body.addEventListener('htmx:afterSwap', (event) => {
	event.detail.target.querySelectorAll('.yaml-code').forEach(el => {
		el.innerHTML = highlightYAML(el.textContent);
	});
});

// JavaScript for dropdown functionality
document.addEventListener('DOMContentLoaded', () => {
	const dropdownButton = document.getElementById('dropdownButton');
	const dropdownMenu = document.getElementById('dropdownMenu');
	const selectedOptionSpan = document.getElementById('selectedOption');

	// Namespace persistence
	const NAMESPACE_KEY = 'tinkerbell-namespace';
	
	// Get namespace from URL, localStorage, cookie, or default to 'All'
	function getCurrentNamespace() {
		const urlParams = new URLSearchParams(window.location.search);
		const urlNamespace = urlParams.get('namespace');
		if (urlNamespace) {
			localStorage.setItem(NAMESPACE_KEY, urlNamespace);
			return urlNamespace;
		}
		
		// Check localStorage first (user's explicit selection takes priority)
		const stored = localStorage.getItem(NAMESPACE_KEY);
		if (stored) {
			return stored;
		}
		
		// Fall back to sa_namespace_display cookie (namespace from service account token)
		// This is only used as initial default when user hasn't made a selection yet
		const cookies = document.cookie.split(';');
		for (let cookie of cookies) {
			const [name, value] = cookie.trim().split('=');
			if (name === 'sa_namespace_display' && value) {
				const namespace = decodeURIComponent(value);
				if (namespace) {
					localStorage.setItem(NAMESPACE_KEY, namespace);
					return namespace;
				}
			}
		}
		
		return ALL_NAMESPACE;
	}
	
	// Save namespace to localStorage
	function saveNamespace(namespace) {
		localStorage.setItem(NAMESPACE_KEY, namespace);
	}
	
	// Build URL with namespace parameter
	function buildUrlWithNamespace(basePath, namespace) {
		if (!namespace || namespace === ALL_NAMESPACE) {
			return basePath;
		}
		const separator = basePath.includes('?') ? '&' : '?';
		return basePath + separator + 'namespace=' + encodeURIComponent(namespace);
	}
	
	// Get the current namespace and update the selector display
	const currentNamespace = getCurrentNamespace();
	if (selectedOptionSpan) {
		selectedOptionSpan.textContent = currentNamespace;
	}

	if (dropdownButton && dropdownMenu) {
		dropdownButton.addEventListener('click', () => {
			dropdownMenu.classList.toggle('hidden');
		});

		// Handle namespace selection
		dropdownMenu.addEventListener('click', (event) => {
			const option = event.target.closest('.namespace-option');
			if (option && option.dataset.value) {
				const namespace = option.dataset.value;
				if (selectedOptionSpan) {
					selectedOptionSpan.textContent = namespace;
				}
				saveNamespace(namespace);
				dropdownMenu.classList.add('hidden');
				
				// Skip navigation for pages where namespace doesn't affect content
				const currentPath = window.location.pathname;
				const baseURL = getBaseURL();
				if (currentPath === baseURL + '/permissions') {
					return;
				}
				
				// Navigate to current page with new namespace
				const newUrl = buildUrlWithNamespace(window.location.pathname, namespace);
				window.location.href = newUrl;
			}
		});

		// Close dropdown when clicking outside
		document.addEventListener('click', (event) => {
			if (!dropdownButton.contains(event.target) && !dropdownMenu.contains(event.target)) {
				dropdownMenu.classList.add('hidden');
			}
		});
	}
	
	// Update all navigation links to include current namespace
	function updateNavLinks() {
		const ns = getCurrentNamespace();
		const navLinks = document.querySelectorAll('nav a[href], .bmc-nav-item');
		navLinks.forEach(link => {
			const href = link.getAttribute('href');
			if (href && !href.startsWith('#') && !href.includes('/hardware/') && !href.includes('/workflows/') && !href.includes('/templates/')) {
				// Only update list view links, not detail view links
				const baseHref = href.split('?')[0];
				link.setAttribute('href', buildUrlWithNamespace(baseHref, ns));
			}
		});
	}
	
	updateNavLinks();

	// BMC dropdown functionality
	const bmcDropdownButton = document.querySelector('[data-collapse-toggle="dropdown-example"]');
	const bmcDropdownMenu = document.getElementById('dropdown-example');

	if (bmcDropdownButton && bmcDropdownMenu) {
		const currentPath = window.location.pathname;
		const isBMCPage = currentPath.startsWith('/bmc/');
		
		// Check localStorage for saved state, default to open if on BMC page
		const savedBMCDropdownState = localStorage.getItem('bmcDropdownOpen');
		const shouldBeOpen = savedBMCDropdownState === 'true' || (savedBMCDropdownState === null && isBMCPage);
		
		if (shouldBeOpen) {
			bmcDropdownMenu.classList.remove('hidden');
			const arrow = bmcDropdownButton.querySelector('svg:last-child');
			arrow.style.transform = 'rotate(180deg)';
		}

		bmcDropdownButton.addEventListener('click', () => {
			bmcDropdownMenu.classList.toggle('hidden');
			const isNowOpen = !bmcDropdownMenu.classList.contains('hidden');
			
			// Save state to localStorage
			localStorage.setItem('bmcDropdownOpen', isNowOpen.toString());

			// Rotate the arrow icon
			const arrow = bmcDropdownButton.querySelector('svg:last-child');
			if (bmcDropdownMenu.classList.contains('hidden')) {
				arrow.style.transform = 'rotate(0deg)';
			} else {
				arrow.style.transform = 'rotate(180deg)';
			}
		});

		// Highlight active BMC navigation item
		const bmcNavItems = document.querySelectorAll('.bmc-nav-item');
		bmcNavItems.forEach(item => {
			const href = item.getAttribute('data-href');
			if (href === currentPath) {
				item.classList.add('bg-tink-teal-100', 'dark:bg-tink-teal-900', 'text-tink-teal-700', 'dark:text-tink-teal-300');
				item.classList.remove('text-gray-700', 'dark:text-gray-300');
			}
		});
	}

	// Workflows dropdown functionality
	const workflowsDropdownButton = document.querySelector('[data-collapse-toggle="workflows-dropdown"]');
	const workflowsDropdownMenu = document.getElementById('workflows-dropdown');

	if (workflowsDropdownButton && workflowsDropdownMenu) {
		const currentPath = window.location.pathname;
		const isWorkflowsPage = currentPath.startsWith('/workflows');
		
		// Check localStorage for saved state, default to open if on workflows page
		const savedWorkflowsDropdownState = localStorage.getItem('workflowsDropdownOpen');
		const shouldBeOpen = savedWorkflowsDropdownState === 'true' || (savedWorkflowsDropdownState === null && isWorkflowsPage);
		
		if (shouldBeOpen) {
			workflowsDropdownMenu.classList.remove('hidden');
			const arrow = workflowsDropdownButton.querySelector('svg');
			if (arrow) arrow.style.transform = 'rotate(180deg)';
		}

		workflowsDropdownButton.addEventListener('click', (e) => {
			e.preventDefault();
			e.stopPropagation();
			workflowsDropdownMenu.classList.toggle('hidden');
			const isNowOpen = !workflowsDropdownMenu.classList.contains('hidden');
			
			// Save state to localStorage
			localStorage.setItem('workflowsDropdownOpen', isNowOpen.toString());

			// Rotate the arrow icon
			const arrow = workflowsDropdownButton.querySelector('svg');
			if (workflowsDropdownMenu.classList.contains('hidden')) {
				if (arrow) arrow.style.transform = 'rotate(0deg)';
			} else {
				if (arrow) arrow.style.transform = 'rotate(180deg)';
			}
		});

		// Highlight active Workflows navigation item
		const workflowsNavItems = document.querySelectorAll('.workflows-nav-item');
		workflowsNavItems.forEach(item => {
			const href = item.getAttribute('data-href');
			if (href === currentPath) {
				item.classList.add('bg-tink-teal-100', 'dark:bg-tink-teal-900', 'text-tink-teal-700', 'dark:text-tink-teal-300');
				item.classList.remove('text-gray-700', 'dark:text-gray-300');
			}
		});
	}

	// Dark mode toggle functionality
	const darkModeToggle = document.getElementById('darkModeToggle');
	const htmlElement = document.documentElement;

	// Check for saved theme preference or default to light mode
	const savedTheme = localStorage.getItem('theme');
	const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;

	if (savedTheme === 'dark' || (!savedTheme && prefersDark)) {
		htmlElement.classList.add('dark');
	} else {
		htmlElement.classList.remove('dark');
	}

	// Toggle dark mode (only if toggle button exists)
	if (darkModeToggle) {
		darkModeToggle.addEventListener('click', () => {
			if (htmlElement.classList.contains('dark')) {
				htmlElement.classList.remove('dark');
				localStorage.setItem('theme', 'light');
			} else {
				htmlElement.classList.add('dark');
				localStorage.setItem('theme', 'dark');
			}
		});
	}

	// Mobile menu functionality
	const mobileMenuButton = document.getElementById('mobileMenuButton');
	const sidebar = document.getElementById('sidebar');
	const mobileMenuOverlay = document.getElementById('mobileMenuOverlay');

	function openMobileMenu() {
		if (sidebar && mobileMenuOverlay) {
			sidebar.classList.remove('-translate-x-full');
			mobileMenuOverlay.classList.remove('hidden');
			document.body.style.overflow = 'hidden';
		}
	}

	function closeMobileMenu() {
		if (sidebar && mobileMenuOverlay) {
			sidebar.classList.add('-translate-x-full');
			mobileMenuOverlay.classList.add('hidden');
			document.body.style.overflow = '';
		}
	}

	if (mobileMenuButton) {
		mobileMenuButton.addEventListener('click', openMobileMenu);
	}

	if (mobileMenuOverlay) {
		mobileMenuOverlay.addEventListener('click', closeMobileMenu);
	}

	// Close mobile menu when window is resized to desktop
	window.addEventListener('resize', () => {
		if (window.innerWidth >= 768) { // md breakpoint
			closeMobileMenu();
		}
	});

	// Update pagination URLs to preserve namespace
	function updatePaginationURLs() {
		const ns = getCurrentNamespace();
		const paginationButtons = document.querySelectorAll('[hx-get*="?page="]');
		paginationButtons.forEach(button => {
			const currentUrl = button.getAttribute('hx-get');
			if (currentUrl && ns && ns !== 'all') {
				// Remove existing namespace if any and add current one
				let baseUrl = currentUrl.split('&namespace=')[0];
				button.setAttribute('hx-get', baseUrl + '&namespace=' + ns);
			}
		});
	}

	// Update pagination URLs after HTMX content swaps
	document.body.addEventListener('htmx:afterSwap', () => {
		updatePaginationURLs();
		updateNavLinks();
	});
	
	// Initial update of pagination URLs on page load
	updatePaginationURLs();

	// Check all functionality
	function setupCheckAllFunctionality() {
		const selectAllMain = document.getElementById('selectAll');
		const selectAllContent = document.getElementById('selectAllContent');
		const rowCheckboxes = document.querySelectorAll('.row-checkbox');

		function updateSelectAllState() {
			const checkedBoxes = document.querySelectorAll('.row-checkbox:checked');
			const allRowCheckboxes = document.querySelectorAll('.row-checkbox');
			
			if (allRowCheckboxes.length === 0) return;
			
			const allChecked = checkedBoxes.length === allRowCheckboxes.length;
			const someChecked = checkedBoxes.length > 0;
			
			// Update both select all checkboxes
			[selectAllMain, selectAllContent].forEach(selectAll => {
				if (selectAll) {
					selectAll.checked = allChecked;
					selectAll.indeterminate = someChecked && !allChecked;
				}
			});
		}

		function handleSelectAll(e) {
			const isChecked = e.target.checked;
			document.querySelectorAll('.row-checkbox').forEach(checkbox => {
				checkbox.checked = isChecked;
			});
		}

		// Attach event listeners to select all checkboxes
		if (selectAllMain) {
			selectAllMain.addEventListener('change', handleSelectAll);
		}
		if (selectAllContent) {
			selectAllContent.addEventListener('change', handleSelectAll);
		}

		// Attach event listeners to row checkboxes
		rowCheckboxes.forEach(checkbox => {
			checkbox.addEventListener('change', updateSelectAllState);
		});

		// Initial state update
		updateSelectAllState();
	}

	// Setup check all functionality on initial load
	setupCheckAllFunctionality();

	// Re-setup check all functionality after HTMX content swaps
	document.body.addEventListener('htmx:afterSwap', () => {
		setupCheckAllFunctionality();
		setupSearchFunctionality();
	});

	// Search functionality
	function setupSearchFunctionality() {
		const globalSearch = document.getElementById('globalSearch');
		const pageSearches = document.querySelectorAll('.page-search');

		// Helper function to filter table rows
		function filterTableRows(searchInput, table) {
			const searchTerm = searchInput.value.toLowerCase().trim();
			const tbody = table.querySelector('tbody');
			if (!tbody) return;
			
			const rows = tbody.querySelectorAll('tr');
			let visibleCount = 0;
			
			rows.forEach(row => {
				const text = row.textContent.toLowerCase();
				const matches = searchTerm === '' || text.includes(searchTerm);
				row.style.display = matches ? '' : 'none';
				if (matches) visibleCount++;
			});
		}

		// Setup page-specific search
		pageSearches.forEach(searchInput => {
			// Find the table - walk up to find a container with a table inside
			let container = searchInput.parentElement;
			let table = null;
			
			// Walk up the DOM to find a sibling or parent container with a table
			while (container && !table) {
				table = container.querySelector('table');
				if (!table) {
					container = container.parentElement;
				}
			}
			
			if (table) {
				searchInput.addEventListener('input', () => {
					filterTableRows(searchInput, table);
				});
				
				// Clear search on Escape key
				searchInput.addEventListener('keydown', (e) => {
					if (e.key === 'Escape') {
						searchInput.value = '';
						filterTableRows(searchInput, table);
					}
				});
			}
		});

		// Setup global search (Quick Navigation)
		if (globalSearch) {
			const resultsContainer = document.getElementById('globalSearchResults');
			let selectedIndex = -1;
			let debounceTimer;
			
			// Icon templates for each resource type
			const icons = {
				hardware: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/></svg>',
				workflow: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"/></svg>',
				template: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>',
				bmc: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01"/></svg>'
			};
			
			// Type colors
			const typeColors = {
				hardware: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
				workflow: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
				template: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
				'bmc-machine': 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
				'bmc-job': 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
				'bmc-task': 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
			};
			
			function showResults(results) {
				if (results.length === 0) {
					resultsContainer.innerHTML = '<div class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">No results found</div>';
					resultsContainer.classList.remove('hidden');
					return;
				}
				
				// Group results by type
				const grouped = {};
				results.forEach(r => {
					if (!grouped[r.typeLabel]) grouped[r.typeLabel] = [];
					grouped[r.typeLabel].push(r);
				});
				
				let html = '';
				let index = 0;
				
				for (const [typeLabel, items] of Object.entries(grouped)) {
					html += '<div class="px-3 py-2 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider bg-gray-50 dark:bg-darkBg/50">' + escapeHtml(typeLabel) + '</div>';
					
					items.forEach(item => {
						const iconHtml = icons[item.icon] || icons.hardware;
						const colorClass = typeColors[item.type] || typeColors.hardware;
						const escapedName = escapeHtml(item.name);
						const escapedNamespace = escapeHtml(item.namespace);
						const escapedTypeLabel = escapeHtml(item.typeLabel);
						const escapedUrl = escapeHtml(item.url);
						html += '<a href="' + escapedUrl + '" class="search-result flex items-center px-4 py-2 hover:bg-gray-100 dark:hover:bg-darkBorder/50 cursor-pointer' + (index === selectedIndex ? ' bg-gray-100 dark:bg-darkBorder/50' : '') + '" data-index="' + index + '">';
						html += '<div class="flex-shrink-0 h-8 w-8 rounded-md bg-tink-teal-50 dark:bg-tink-teal-900/30 flex items-center justify-center text-tink-teal-600 dark:text-tink-teal-400 mr-3">' + iconHtml + '</div>';
						html += '<div class="flex-1 min-w-0 overflow-hidden">';
						html += '<div class="text-sm font-medium text-gray-900 dark:text-white" title="' + escapedName + '">' + escapedName + '</div>';
						html += '<div class="text-xs text-gray-500 dark:text-gray-400">' + escapedNamespace + '</div>';
						html += '</div>';
						html += '<span class="ml-3 flex-shrink-0 px-2 py-0.5 rounded text-xs font-medium whitespace-nowrap ' + colorClass + '">' + escapedTypeLabel + '</span>';
						html += '</a>';
						index++;
					});
				}
				
				resultsContainer.innerHTML = html;
				resultsContainer.classList.remove('hidden');
			}
			
			function hideResults() {
				resultsContainer.classList.add('hidden');
				selectedIndex = -1;
			}
			
			function updateSelection() {
				const items = resultsContainer.querySelectorAll('.search-result');
				items.forEach((item, i) => {
					if (i === selectedIndex) {
						item.classList.add('bg-gray-100', 'dark:bg-darkBorder/50');
						item.scrollIntoView({ block: 'nearest' });
					} else {
						item.classList.remove('bg-gray-100', 'dark:bg-darkBorder/50');
					}
				});
			}
			
			async function performSearch() {
				const query = globalSearch.value.trim();
				if (query.length < 1) {
					hideResults();
					return;
				}
				
				const ns = getCurrentNamespace();
				const baseURL = getBaseURL();
				const url = baseURL + '/api/search?q=' + encodeURIComponent(query) + (ns && ns !== 'all' ? '&namespace=' + encodeURIComponent(ns) : '');
				
				try {
					const response = await fetch(url);
					const data = await response.json();
					selectedIndex = -1;
					showResults(data.results);
				} catch (error) {
					console.error('Search error:', error);
					hideResults();
				}
			}
			
			// Debounced search on input
			globalSearch.addEventListener('input', () => {
				clearTimeout(debounceTimer);
				debounceTimer = setTimeout(performSearch, 150);
			});
			
			// Keyboard navigation
			globalSearch.addEventListener('keydown', (e) => {
				const items = resultsContainer.querySelectorAll('.search-result');
				
				if (e.key === 'ArrowDown') {
					e.preventDefault();
					if (selectedIndex < items.length - 1) {
						selectedIndex++;
						updateSelection();
					}
				} else if (e.key === 'ArrowUp') {
					e.preventDefault();
					if (selectedIndex > 0) {
						selectedIndex--;
						updateSelection();
					}
				} else if (e.key === 'Enter') {
					e.preventDefault();
					if (selectedIndex >= 0 && items[selectedIndex]) {
						window.location.href = items[selectedIndex].getAttribute('href');
					}
				} else if (e.key === 'Escape') {
					globalSearch.value = '';
					hideResults();
					globalSearch.blur();
				}
			});
			
			// Focus search on input click
			globalSearch.addEventListener('focus', () => {
				if (globalSearch.value.trim().length > 0) {
					performSearch();
				}
			});
			
			// Hide results when clicking outside
			document.addEventListener('click', (e) => {
				if (!globalSearch.contains(e.target) && !resultsContainer.contains(e.target)) {
					hideResults();
				}
			});
			
			// Global keyboard shortcut (Ctrl+K or Cmd+K)
			document.addEventListener('keydown', (e) => {
				if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
					e.preventDefault();
					globalSearch.focus();
					globalSearch.select();
				}
			});
		}
	}
	
	// Setup search functionality on initial load
	setupSearchFunctionality();

	// User Profile Dropdown functionality
	window.addEventListener('load', () => {
		const currentPath = window.location.pathname;

		// User Profile Dropdown functionality
		const userProfileButton = document.getElementById('userProfileButton');
		const userProfileDropdown = document.getElementById('userProfileDropdown');
		
		if (userProfileButton && userProfileDropdown) {
			// Toggle dropdown
			userProfileButton.addEventListener('click', (e) => {
				e.stopPropagation();
				const isHidden = userProfileDropdown.classList.contains('hidden');
				if (isHidden) {
					userProfileDropdown.classList.remove('hidden');
					userProfileButton.setAttribute('aria-expanded', 'true');
				} else {
					userProfileDropdown.classList.add('hidden');
					userProfileButton.setAttribute('aria-expanded', 'false');
				}
			});
			
			// Close dropdown when clicking outside
			document.addEventListener('click', (e) => {
				if (!userProfileButton.contains(e.target) && !userProfileDropdown.contains(e.target)) {
					userProfileDropdown.classList.add('hidden');
					userProfileButton.setAttribute('aria-expanded', 'false');
				}
			});
			
			// Display API server URL from display-only cookie
			// This cookie is set by the server during login and is NOT used for API communication
			const apiServerDisplay = document.getElementById('apiServerDisplay');
			const apiServerContainer = document.getElementById('apiServerDisplayContainer');
			const getCookie = (name) => {
				const value = `; ${document.cookie}`;
				const parts = value.split(`; ${name}=`);
				if (parts.length === 2) return decodeURIComponent(parts.pop().split(';').shift());
				return '';
			};
			const apiServer = getCookie('apiserver_display');
			if (apiServerDisplay && apiServerContainer && apiServer) {
				apiServerDisplay.textContent = apiServer;
				apiServerContainer.classList.remove('hidden');
			}
		}

		// Handle logout button
		const logoutButton = document.getElementById('logoutButton');
		if (logoutButton) {
			logoutButton.addEventListener('click', async (e) => {
				e.preventDefault();
				const baseURL = getBaseURL();
				try {
					await fetch(baseURL + '/api/auth/logout', { method: 'POST' });
				} catch (error) {
					console.error('Logout error:', error);
				}
				// Redirect to login page (session cleared via HttpOnly cookies by server)
				window.location.href = baseURL + '/login';
			});
		}
	});
});
