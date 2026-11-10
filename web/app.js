/* SubGate operations console. No third-party scripts or persistent auth tokens. */
(() => {
  'use strict';

  const $ = (selector, root = document) => root.querySelector(selector);
  const app = $('#app');
  const modal = $('#modal');
  const modalForm = $('#modal-form');
  const modalContent = $('#modal-content');
  const number = new Intl.NumberFormat('ru-RU');
  const state = {
    user: null, status: null, route: 'overview', data: {}, filters: {},
    loading: false, controller: null, poll: null, stream: null, revision: 0,
    toastTimer: null, searchTimer: null, chartCleanup: null, updated: null, dialogRevision: 0, configEditor: null,
  };

  const paths = {
    overview: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
    live: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
    nodes: '<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M16 6.5h2M16 17.5h2"/>',
    users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M22 21v-2a4 4 0 0 0-3-3.87"/><circle cx="9" cy="7" r="4"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
    agents: '<path d="m12 3 8 4v10l-8 4-8-4V7l8-4ZM4 7l8 4 8-4M12 11v10"/>',
    analysis: '<path d="M4 6h16M4 12h16M4 18h16"/><circle cx="8" cy="6" r="2"/><circle cx="16" cy="12" r="2"/>',
    incidents: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a18 18 0 0 1 0 18 18 18 0 0 1 0-18Z"/>',
    activity: '<path d="M8 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-3M8 7h6M7 12h6M7 17h4M16 3h5v5M21 3l-8 8"/>',
    ipintel: '<circle cx="10" cy="10" r="7"/><path d="m15 15 6 6M10 7v6M7 10h6"/>',
    alerts: '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/>',
    backups: '<path d="M4 4h16v5H4zM5 9v11h14V9M9 13h6"/>',
    audit: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6ZM14 2v6h6M8 13h8M8 17h6"/>',
    settings: '<path d="M4 6h16M4 12h16M4 18h16"/><circle cx="8" cy="6" r="2"/><circle cx="16" cy="12" r="2"/><circle cx="10" cy="18" r="2"/>',
    refresh: '<path d="M21 3v6h-6M3 21v-6h6M3 9a9 9 0 0 1 15-5l3 5M21 15a9 9 0 0 1-15 5l-3-5"/>',
    arrow: '<path d="M5 12h14m-5-5 5 5-5 5"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    download: '<path d="M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5"/>',
    menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
    logout: '<path d="M9 3H4v18h5M9 12h12m-4-4 4 4-4 4"/>',
    check: '<path d="m5 12 4 4L19 6"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    error: '<path d="m12 3 10 18H2L12 3ZM12 9v4M12 17h.01"/>',
  };
  const pages = {
    overview: ['Обзор', 'Что требует внимания: состояние нод, полнота телеметрии и наблюдения Xray.'],
    live: ['Live-трафик', 'Текущая скорость по аккаунтам и нодам. Только свежие измерения агентов.'],
    nodes: ['Ноды', 'Состояние и протоколы нод, синхронизированных с Remnawave.'],
    agents: ['Агенты', 'Коллекторы Xray, состояние сбора, очереди и диагностика.'],
    incidents: ['Разбор аномалий', 'Наблюдения по журналам Xray: измерения, контекст и ваше решение.'],
    analysis: ['Правила анализа', 'Xray, nDPI, длительная перегрузка и временные ограничения.'],
    activity: ['Журнал Xray', 'Источники и назначения соединений. Доступ и экспорт фиксируются в аудите.'],
    ipintel: ['IP Intelligence', 'Проверка IPv4 и IPv6 по локальным источникам IP-данных.'],
    alerts: ['Оповещения', 'Реестр событий и заметок для оператора.'],
    backups: ['Резервные копии', 'Зашифрованные согласованные снимки базы данных SubGate.'],
    audit: ['Аудит', 'История входов, решений и изменений конфигурации.'],
    settings: ['Настройки', 'Конфигурация Core и состояние компонентов.'],
  };
  const schemas = {
    alerts: {
      sample: { name: 'Заметка оператора', severity: 'info', message: '', node_id: '', created_at: new Date().toISOString() },
      help: 'name, message, severity (info, warning, critical), node_id и created_at описывают событие. Это реестр заметок; автоматическая отправка уведомлений и вычисление условий в этой версии не подключены.',
    },
  };

  function esc(value) {
    return String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
  }
  function icon(name) { return `<svg class="icon" viewBox="0 0 24 24" aria-hidden="true">${paths[name] || paths.overview}</svg>`; }
  function finite(value) { const n = Number(value); return Number.isFinite(n) ? n : 0; }
  function count(value) { return value == null ? '—' : number.format(finite(value)); }
  function bytes(value) {
    if (value == null) return '—';
    let n = Math.max(0, finite(value));
    const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ'];
    let i = 0;
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    return `${number.format(Number(n.toFixed(i ? 1 : 0)))} ${units[i]}`;
  }
  function speed(value) {
    if (value == null) return '—';
    let n = Math.max(0, finite(value));
    const units = ['бит/с', 'Кбит/с', 'Мбит/с', 'Гбит/с', 'Тбит/с'];
    let i = 0;
    while (n >= 1000 && i < units.length - 1) { n /= 1000; i++; }
    return `${number.format(Number(n.toFixed(i ? 2 : 0)))} ${units[i]}`;
  }
  function parseDate(value) {
    if (!value || String(value).startsWith('0001-')) return null;
    let normalized = value;
    if (typeof value === 'string' && /^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d$/.test(value)) normalized = value.replace(' ', 'T') + 'Z';
    const date = new Date(normalized);
    return Number.isNaN(date.getTime()) ? null : date;
  }
  function date(value, compact = false) {
    const parsed = parseDate(value);
    return parsed ? parsed.toLocaleString('ru-RU', compact ? { hour: '2-digit', minute: '2-digit', second: '2-digit' } : { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—';
  }
  function duration(seconds) {
    const n = Math.max(0, finite(seconds));
    if (n >= 86400) return `${Math.floor(n / 86400)} д ${Math.floor(n % 86400 / 3600)} ч`;
    if (n >= 3600) return `${Math.floor(n / 3600)} ч ${Math.floor(n % 3600 / 60)} мин`;
    return `${Math.floor(n / 60)} мин`;
  }
  function canWrite() { return Boolean(state.user); }
  function principal(value) { return { ID: value.id ?? value.ID, Username: String(value.username ?? value.Username ?? '') }; }
  function badge(status, online) {
    const key = String(status || 'unknown').toLowerCase();
    const labels = { online: 'Онлайн', active: 'Активен', offline: 'Офлайн', disabled: 'Отключён', expired: 'Истёк', limited: 'Лимит', error: 'Ошибка', revoked: 'Отозван', unknown: 'Нет данных', warning: 'Внимание', critical: 'Критично', info: 'Информация', success: 'Успешно', failure: 'Ошибка', accepted: 'Разрешено', rejected: 'Отклонено' };
    const healthy = !['error', 'critical', 'failure', 'rejected'].includes(key) && (online === true || ['online', 'active', 'success', 'accepted'].includes(key));
    const tone = healthy ? '' : ['error', 'critical', 'failure'].includes(key) ? 'red' : ['warning', 'limited', 'expired'].includes(key) ? 'amber' : 'neutral';
    return `<span class="badge ${tone}">${esc(labels[key] || status || 'Нет данных')}</span>`;
  }
  function empty(title, detail, name = state.route, action = '') {
    return `<div class="empty">${icon(name)}<strong>${esc(title)}</strong><p>${esc(detail)}</p>${action}</div>`;
  }
  function table(headers, rows, options = {}) {
    if (!rows.length) return empty(options.title || 'Пока нет данных', options.detail || 'Данные появятся после подключения и синхронизации.', options.icon);
    return `<div class="table-wrap"><table><thead><tr>${headers.map(item => `<th scope="col">${esc(item)}</th>`).join('')}</tr></thead><tbody>${rows.map(row => `<tr>${row.map(cell => `<td>${cell}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
  }
  function panel(title, body, aside = '') { return `<section class="panel"><div class="panel-head"><h2>${esc(title)}</h2>${aside}</div>${body}</section>`; }
  function metric(title, value, foot, glyph, suffix = '') {
    return `<article class="metric" data-tone="${esc(glyph)}"><div class="metric-head">${esc(title)}${icon(glyph)}</div><div class="metric-value">${esc(value)}${suffix ? `<small>${esc(suffix)}</small>` : ''}</div><div class="metric-foot">${esc(foot)}</div></article>`;
  }
  function filters() { return state.filters[state.route] || (state.filters[state.route] = { q: '', offset: 0 }); }
  function actionButton(action, text, extras = '', classes = '') {
    return `<button type="button" class="${classes}" data-action="${action}" ${extras}>${esc(text)}</button>`;
  }
  function refreshButton() { return `<button type="button" data-action="refresh" aria-label="Обновить данные">${icon('refresh')}Обновить</button>`; }
  function pageHead(actions = '') {
    const [title, description] = pages[state.route];
    return `<header class="page-head"><div><div class="eyebrow">SubGate / Мониторинг</div><h1>${esc(title)}</h1><p>${esc(description)}</p></div><div class="head-actions">${actions}${refreshButton()}</div></header>`;
  }
  function toolbar(placeholder) {
    return `<div class="toolbar"><label class="search-field"><span class="sr-only">${esc(placeholder)}</span><input id="page-search" type="search" autocomplete="off" data-filter="q" value="${esc(filters().q)}" placeholder="${esc(placeholder)}"></label><small>${state.loading ? 'Обновление…' : 'Обновляется автоматически'}</small></div>`;
  }
  function pageFoot(total, loaded) {
    const offset = finite(filters().offset);
    return `<div class="table-foot"><span>${total ? `${count(offset + 1)}–${count(offset + loaded)} из ${count(total)}` : '0 записей'}</span><div class="pager">${actionButton('previous', 'Назад', offset === 0 ? 'disabled' : '')}${actionButton('next', 'Далее', offset + loaded >= total ? 'disabled' : '')}</div></div>`;
  }

  class APIError extends Error {
    constructor(message, status) { super(message); this.name = 'APIError'; this.status = status; }
  }
  async function request(path, options = {}) {
    const method = options.method || 'GET';
    const headers = { Accept: 'application/json' };
    if (method !== 'GET' && method !== 'HEAD') headers['X-SubGate-CSRF'] = '1';
    if (options.body !== undefined) headers['Content-Type'] = 'application/json';
    let response;
    try {
      response = await fetch(`/api/v1${path}`, {
        credentials: 'same-origin', cache: 'no-store', ...options, method, headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
      });
    } catch (error) {
      if (error.name === 'AbortError') throw error;
      throw new APIError('Не удалось связаться с Core. Проверьте соединение.', 0);
    }
    if (!response.ok) {
      const payload = await response.json().catch(() => ({}));
      throw new APIError(payload.error || `Ошибка сервера: HTTP ${response.status}`, response.status);
    }
    return options.raw ? response : response.json();
  }
  function toast(message, error = false) {
    const element = $('#toast');
    clearTimeout(state.toastTimer);
    element.textContent = message;
    element.className = `visible${error ? ' error' : ''}`;
    state.toastTimer = setTimeout(() => { element.className = ''; }, error ? 7000 : 4000);
  }
  function handleError(error) {
    if (error.name === 'AbortError') return;
    if (error.status === 401 && state.user) {
      state.user = null;
      stopPage();
      closeDialog();
      renderLogin('Сессия завершилась. Войдите снова.');
    } else toast(error.message, true);
  }
  function stopPage() {
    clearTimeout(state.poll);
    clearTimeout(state.searchTimer);
    state.controller?.abort();
    state.stream?.close();
    state.stream = null;
    state.revision++;
    state.loading = false;
    if (state.chartCleanup) state.chartCleanup();
    state.chartCleanup = null;
  }
  function routeFromHash() {
    const hash = location.hash.replace(/^#\/?/, '');
    const [candidate, query] = hash.split('?');
    const route = Object.hasOwn(pages, candidate) ? candidate : 'overview';
    if (route === 'activity' && query) {
      const params = new URLSearchParams(query);
      const localTime = value => {
        const parsed = parseDate(value);
        if (!parsed) return '';
        return new Date(parsed.getTime() - parsed.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
      };
      state.filters.activity = { user: params.get('user') || '', node: params.get('node') || '', from: localTime(params.get('from')), to: localTime(params.get('to')), q: '', offset: 0 };
    }
    return route === 'activity' && !canWrite() ? 'overview' : route;
  }
  function renderShell() {
    const groups = [
      ['НАБЛЮДЕНИЕ', ['overview', 'live', 'nodes', 'agents']],
      ['АНАЛИЗ', ['incidents', 'activity', 'ipintel', 'analysis']],
      ['СЕРВИС', ['alerts', 'backups', 'audit', 'settings']],
    ];
    app.innerHTML = `<a class="skip-link" href="#main-content">Перейти к содержимому</a><aside class="sidebar" id="sidebar" aria-label="Главное меню"><a class="brand" href="#/overview"><span class="logo-mark">S</span>SubGate<small>CORE</small></a><div class="workspace"><span class="workspace-avatar">${icon('nodes')}</span><span><strong>Мониторинг VPN</strong><small>Ноды · Xray · Разбор аномалий</small></span></div><nav class="nav">${groups.map(([label, routes]) => `<div class="nav-label">${label}</div>${routes.filter(route => route !== 'activity' || canWrite()).map(route => `<a href="#/${route}" data-route="${route}">${icon(route)}${pages[route][0]}</a>`).join('')}`).join('')}</nav><div class="sidebar-foot"><span class="status-dot" id="connection-dot"></span><span id="connection-label">Core подключён</span><small class="sidebar-version">SubGate · <span id="sidebar-version">—</span></small></div></aside><div class="shell"><div class="topbar"><div class="crumb"><button class="menu-button" type="button" data-action="menu" aria-label="Открыть меню" aria-controls="sidebar" aria-expanded="false">${icon('menu')}</button><span class="root-crumb">Инфраструктура</span><span id="breadcrumb"></span></div><div class="top-right"><span class="clock" id="clock"></span><span class="avatar" title="Владелец">${esc(state.user.Username.slice(0, 2).toUpperCase())}</span><span>${esc(state.user.Username)}</span><button type="button" class="ghost" data-action="logout" aria-label="Выйти из аккаунта" title="Выйти">${icon('logout')}</button></div></div><main class="content" id="main-content" tabindex="-1"></main></div>`;
    updateShell();
  }
  function updateShell() {
    const crumb = $('#breadcrumb');
    if (!crumb) return;
    crumb.textContent = pages[state.route][0];
    document.title = `${pages[state.route][0]} · SubGate`;
    document.querySelectorAll('[data-route]').forEach(link => {
      const active = link.dataset.route === state.route;
      link.classList.toggle('active', active);
      if (active) link.setAttribute('aria-current', 'page'); else link.removeAttribute('aria-current');
    });
    $('#clock').textContent = new Date().toLocaleString('ru-RU', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });
    $('#sidebar-version').textContent = state.status?.version || '—';
    $('#connection-label').textContent = state.status?.sync_error ? 'Ошибка синхронизации' : 'Core подключён';
    $('#connection-dot').classList.toggle('warning-dot', !!state.status?.sync_error);
  }
  async function navigate() {
    if (!state.user) return;
    stopPage();
    closeDialog();
    state.route = routeFromHash();
    state.data = {};
    $('#sidebar')?.classList.remove('open');
    $('[data-action="menu"]')?.setAttribute('aria-expanded', 'false');
    updateShell();
    renderPage(`<div class="loading" role="status">Подключение к данным…</div>`);
    await loadPage();
  }
  function schedulePoll() {
    clearTimeout(state.poll);
    state.poll = setTimeout(() => {
      if (!state.user) return;
      if (document.hidden || modal.open || state.loading) { schedulePoll(); return; }
      loadPage(true);
    }, ['overview', 'live'].includes(state.route) ? 5000 : 30000);
  }
  function startLiveStream(revision) {
    if (typeof EventSource === 'undefined' || state.stream || state.route !== 'live') return;
    const stream = new EventSource('/api/v1/live/stream');
    state.stream = stream;
    stream.addEventListener('live', event => {
      if (revision !== state.revision || state.route !== 'live') { stream.close(); return; }
      try {
        const payload = JSON.parse(event.data);
        state.data.live = payload;
        state.updated = new Date();
        renderPage();
      } catch (_) { /* a malformed frame is ignored; the next frame is complete */ }
    });
    stream.onerror = () => {
      if (state.stream === stream) state.stream = null;
      stream.close();
    };
  }
  async function loadPage(background = false) {
    if (!state.user) return;
    state.controller?.abort();
    const controller = new AbortController();
    state.controller = controller;
    const revision = ++state.revision;
    const route = state.route;
    const f = { ...filters() };
    const get = path => request(path, { signal: controller.signal });
    state.loading = true;
    $('[data-action="refresh"]')?.setAttribute('disabled', '');
    try {
      let data;
      const query = new URLSearchParams({ limit: '50', offset: String(f.offset || 0), q: f.q || '' });
      if (route === 'overview') {
        const [status, nodes, agents, live, traffic, alerts] = await Promise.all([get('/system/status'), get('/nodes?limit=1000'), get('/agents'), get('/live'), get('/traffic?minutes=60'), get('/alerts?limit=5')]);
        data = { status, nodes, agents, live, traffic, alerts };
      } else if (route === 'live') {
        const [status, live, traffic] = await Promise.all([get('/system/status'), get('/live'), get(`/traffic?minutes=${f.minutes || 60}`)]);
        data = { status, live, traffic };
      } else if (route === 'agents') {
        const [list, nodes, status] = await Promise.all([get('/agents'), get('/nodes?limit=1000'), get('/system/status')]);
        data = { list, nodes, status };
      } else if (route === 'nodes') {
        data = { list: await get(`/nodes?${query}`) };
      } else if (route === 'incidents') {
        data = { list: await get('/incidents?limit=200') };
      } else if (route === 'analysis') {
        const [policy, shaper, monitor] = await Promise.all([get('/analysis/policy'), get('/analysis/shaper'), get('/analysis/monitor')]);
        data = { ...policy, shaper, monitor };
      } else if (route === 'activity') {
        data = { list: await get(`/activity?${activityQuery(f)}`) };
      } else if (route === 'ipintel') {
        data = { ...state.data, status: await get('/system/status') };
      } else if (route === 'settings') {
        const [settings, status] = await Promise.all([get('/settings'), get('/system/status')]);
        data = { settings, status };
      } else if (route === 'backups' || route === 'audit') {
        data = { list: await get(`/${route}?limit=200`) };
      } else {
        data = { list: await get(`/${route}?${query}`) };
      }
      if (revision !== state.revision || route !== state.route) return;
      state.data = data;
      if (data.status) state.status = data.status;
      state.updated = new Date();
      state.loading = false;
      updateShell();
      renderPage();
      if (route === 'live') startLiveStream(revision);
    } catch (error) {
      if (revision !== state.revision || error.name === 'AbortError') return;
      state.loading = false;
      if (error.status === 401) { handleError(error); return; }
      if (background && Object.keys(state.data).length) {
        toast(`Данные не обновились: ${error.message}`, true);
        const foot = $('#updated-at');
        if (foot) foot.textContent = `Последнее успешное обновление ${date(state.updated, true)} · соединение потеряно`;
      } else {
        renderPage(`<div class="notice error" role="alert">${esc(error.message)}</div>${empty('Не удалось загрузить данные', 'Проверьте доступность Core и повторите запрос.', 'error', '<button type="button" data-action="refresh">Повторить</button>')}`);
      }
    } finally {
      if (revision === state.revision && state.user) {
        state.loading = false;
        $('[data-action="refresh"]')?.removeAttribute('disabled');
        schedulePoll();
      }
    }
  }

  function renderPage(override) {
    const main = $('#main-content');
    if (!main) return;
    const focused = document.activeElement;
    const focusID = main.contains(focused) ? focused.id : null;
    const selection = focusID && typeof focused.selectionStart === 'number' ? [focused.selectionStart, focused.selectionEnd] : null;
    if (state.chartCleanup) state.chartCleanup();
    state.chartCleanup = null;
    const functions = { overview: overviewPage, live: livePage, nodes: nodesPage, agents: agentsPage, incidents: incidentsPage, analysis: analysisPage, activity: activityPage, ipintel: ipintelPage, backups: backupsPage, audit: auditPage, settings: settingsPage };
    const body = override || (functions[state.route] ? functions[state.route]() : configPage());
    const demo = state.status?.demo ? '<div class="demo-banner">' + icon('error') + '<strong>DEMO</strong><span>Демонстрационные ноды и пользователи. Синтетический трафик не генерируется; измерения появятся после подключения агента.</span></div>' : '';
    main.innerHTML = (override ? pageHead() : '') + demo + body + `<footer class="footer"><span>SubGate · ${esc(state.status?.version || 'Core')} · Владелец</span><span id="updated-at">${state.updated ? `Обновлено ${date(state.updated, true)}` : ''}</span></footer>`;
    if (focusID) {
      const replacement = document.getElementById(focusID);
      if (replacement) {
        replacement.focus({ preventScroll: true });
        if (selection && replacement.setSelectionRange) { try { replacement.setSelectionRange(...selection); } catch (_) { /* datetime and select do not support selection */ } }
      }
    }
    const canvas = $('#traffic-chart');
    if (canvas) drawChart(canvas, state.data.traffic?.data || []);
  }

  function chartPanel() {
    const raw = state.data.traffic?.data || [];
    const grouped = new Map();
    raw.forEach(point => {
      const key = point.timestamp;
      const current = grouped.get(key) || { timestamp: key, uplink_bps: 0, downlink_bps: 0 };
      current.uplink_bps += Math.max(0, finite(point.uplink_bps ?? point.uplink_avg_bps));
      current.downlink_bps += Math.max(0, finite(point.downlink_bps ?? point.downlink_avg_bps));
      grouped.set(key, current);
    });
    const data = [...grouped.values()].sort((a, b) => String(a.timestamp).localeCompare(String(b.timestamp)));
    return panel('Трафик инфраструктуры', `<div class="chart"><canvas id="traffic-chart" role="img" aria-label="История скорости исходящего и входящего трафика в битах в секунду. ${data.length ? `${data.length} измерений за выбранный период.` : 'Измерений пока нет.'}"></canvas>${data.length ? '' : `<div class="chart-empty">${icon('live')}<span>Нет измерений за выбранный период</span><small>Подключите агент и включите Xray StatsService</small></div>`}</div><div class="chart-caption"><span id="chart-reading">${data.length ? `${count(data.length)} точек · шаг 10 секунд` : 'Только реальные данные агентов'}</span><span>бит/с</span></div>`, '<div class="legend"><span>Исходящий</span><span>Входящий</span></div>');
  }
  function overviewPage() {
    const { status, nodes, agents, live, alerts } = state.data;
    if (!status) return '';
    const onlineNodes = (nodes.data || []).filter(node => node.status === 'online').length;
    const sourceErrors = (agents.data || []).filter(agent => agent.status !== 'revoked' && (!agent.online || agent.diagnostics?.stats_error || agent.diagnostics?.logs_error)).length;
    const health = `<div class="panel-body"><div class="health-item"><span>Core</span>${badge('online')}</div><div class="health-item"><span>Связь с Remnawave</span>${badge(status.sync_error ? 'error' : parseDate(status.last_sync) ? 'success' : 'unknown')}</div><div class="health-item"><span>Синхронизация</span><strong>${esc(date(status.last_sync))}</strong></div><div class="health-item"><span>Источники с проблемами</span><strong class="${sourceErrors ? 'negative' : 'positive'}">${count(sourceErrors)}</strong></div><div class="health-item"><span>Записей Xray сохранено</span><strong>${count(status.activity_events)}</strong></div><div class="health-item"><span>Uptime Core</span><strong>${esc(duration(status.uptime_seconds))}</strong></div></div>`;
    const alertBody = (alerts.data || []).length ? `<div class="alert-strip">${alerts.data.map(item => `<article><div>${badge(item.severity || 'info')}<strong>${esc(item.name)}</strong></div><p>${esc(item.message || '')}</p></article>`).join('')}</div>` : empty('Оповещений нет', 'В реестре нет текущих событий.', 'alerts');
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="sync">${icon('refresh')}Синхронизировать</button>` : '') +
      `<div class="metrics">${metric('Ноды в инфраструктуре', count(status.nodes), `${count(onlineNodes)} онлайн по данным панели`, 'nodes')}${metric('Ожидают разбора', count(status.incidents_new), 'Наблюдения Xray, требующие вашего решения', 'alerts')}${metric('Агенты онлайн', count(status.agents_online), `Из ${count((agents.data || []).length)} подключённых коллекторов`, 'agents')}${metric('Потоки телеметрии', count((live.data || []).length), `↓ ${speed(live.downlink_bps)} · ↑ ${speed(live.uplink_bps)}`, 'live')}</div>` +
      `<div class="attention-grid"><a class="attention-card" href="#/incidents">${icon('alerts')}<span><strong>Разобрать аномалии</strong><small>${count(status.incidents_new)} новых наблюдений · признаки и решения</small></span>${icon('arrow')}</a><a class="attention-card violet" href="#/agents">${icon('agents')}<span><strong>Проверить источники</strong><small>${sourceErrors ? count(sourceErrors) + ' агентов требуют внимания' : 'Диагностика сбора, очереди и пропуски'}</small></span>${icon('arrow')}</a></div>` +
      (status.sync_error ? `<div class="notice error">Синхронизация с панелью: ${esc(status.sync_error)}</div>` : '') +
      `<div class="grid-main">${chartPanel()}${panel('Состояние системы', health, '<a class="view-link" href="#/settings">Подробнее ' + icon('arrow') + '</a>')}</div>` +
      panel('Флот нод', nodeTable((nodes.data || []).slice(0, 8), false), `<a class="view-link" href="#/nodes">Все ноды ${icon('arrow')}</a>`) +
      `<div class="section-gap">${panel('Оповещения', alertBody, `<a class="view-link" href="#/alerts">Реестр ${icon('arrow')}</a>`)}</div>`;
  }
  function nodeTable(nodes, actions = true) {
    const rows = nodes.map(node => [
      `<div class="row-name"><span class="country-tile">${esc(node.country || '—')}</span><div><strong>${esc(node.name || node.id)}</strong><small class="mono">${esc(node.id)}</small></div></div>`,
      `<span class="mono">${esc(node.ip_address || '—')}</span>`, badge(node.status),
      `<strong>${count(node.users_online)}</strong>`, bytes(node.traffic_used_bytes),
      (node.inbounds || []).map(inbound => `<span class="tag" title="${esc(`${inbound.tag || ''} · ${inbound.network || ''} · ${inbound.security || ''} · :${inbound.port || ''}`)}">${esc(inbound.type || inbound.tag || 'inbound')}</span>`).join('') || '—',
      actions ? resourceActions('nodes', node) : `<a href="#/nodes" class="view-link">Открыть ${icon('arrow')}</a>`,
    ]);
    return table(['Нода', 'Адрес', 'Состояние', 'Онлайн', 'Трафик', 'Протоколы', 'Действия'], rows, { title: 'Ноды ещё не синхронизированы', detail: 'Укажите адрес Remnawave в конфигурации Core и запустите синхронизацию.', icon: 'nodes' });
  }
  function resourceActions(kind, item) {
    const common = `data-kind="${kind}" data-id="${esc(item.id)}"`;
    const disabled = String(item.status).toLowerCase() === 'disabled';
    const nodeActions = kind === 'nodes' && canWrite()
      ? actionButton('resource-action', disabled ? 'Включить' : 'Отключить', `${common} data-operation="${disabled ? 'enable' : 'disable'}"`) + actionButton('resource-action', 'Рестарт', `${common} data-operation="restart"`) + actionButton('resource-action', 'Сброс трафика', `${common} data-operation="reset-traffic"`)
      : '';
    return `<div class="table-actions">${actionButton('details', 'Детали', common)}${nodeActions}${kind === 'users' && canWrite() ? `<a class="row-link" href="#/activity?user=${encodeURIComponent(item.id)}">Журнал</a>` : ''}</div>`;
  }
  function nodesPage() {
    const list = state.data.list || { data: [], total: 0 };
    return pageHead(canWrite() ? '<button type="button" class="primary" data-action="sync">' + icon('refresh') + 'Синхронизировать</button>' : '') + toolbar('Поиск по имени, IP, стране или протоколу') + panel('Ноды Remnawave', nodeTable(list.data || []) + pageFoot(list.total || 0, (list.data || []).length), `<small>${count(list.total)} нод</small>`) + '<p class="help-line">Состояние и число пользователей онлайн берутся из Remnawave при каждой синхронизации (раз в минуту).</p>';
  }

  function livePage() {
    const live = state.data.live || {};
    const q = String(filters().q || '').toLowerCase();
    const all = live.data || [];
    const data = all.filter(item => !q || `${item.user_id} ${item.user_name || ''} ${item.user_email || ''} ${item.node_id} ${item.node_name || ''} ${item.scope || ''}`.toLowerCase().includes(q));
    const totalUp = all.reduce((sum, item) => sum + finite(item.uplink_bps), 0);
    const totalDown = all.reduce((sum, item) => sum + finite(item.downlink_bps), 0);
    const coarse = all.filter(item => item.coarse || item.scope === 'inbound').length;
    const liveCell = (item, direction) => {
      const current = finite(item[`${direction}_bps`]);
      const ema3 = finite(item[`${direction}_ema_bps`]);
      const ema10 = finite(item[`${direction}_ema10_bps`]);
      const avg = finite(item[`${direction}_avg_bps`]);
      const peak = finite(item[`${direction}_peak_bps`]);
      const p95 = finite(item[`${direction}_p95_bps`]);
      return `<div class="live-cell"><strong class="${direction === 'uplink' ? 'positive' : ''}">${direction === 'uplink' ? '↑' : '↓'} ${esc(speed(current))}</strong><small>EMA 3s ${esc(speed(ema3))} · 10s ${esc(speed(ema10))}</small><small>avg ${esc(speed(avg))} · peak ${esc(speed(peak))} · p95 ${esc(speed(p95))}</small></div>`;
    };
    const rows = data.map(item => {
      const inbound = item.scope === 'inbound' || item.coarse;
      const userLabel = inbound ? `⚠ ${item.user_id || 'inbound'}` : (item.user_name || item.user_email || item.panel_user_id || item.user_id);
      const userDetail = inbound ? 'coarse: inbound tag, не per-user' : (item.user_email || item.panel_user_id || item.user_id);
      const nodeLabel = item.node_name || item.node_id;
      const nodeDetail = [item.node_address, item.node_id].filter(Boolean).join(' · ');
      const history = inbound ? '<span class="muted">не применимо</span>' : actionButton('user-traffic', 'История', `data-user="${esc(item.panel_user_id || item.user_id)}"`);
      return [
        `<strong>${esc(userLabel)}</strong><small class="mono">${esc(userDetail)}</small>`,
        `<strong>${esc(nodeLabel)}</strong><small class="mono">${esc(nodeDetail)}</small>`,
        liveCell(item, 'uplink'), liveCell(item, 'downlink'),
        `<strong>↑ ${esc(bytes(item.uplink_delta_bytes))}</strong><small>↓ ${esc(bytes(item.downlink_delta_bytes))}</small><small>24ч: ↑ ${esc(bytes(item.uplink_total_bytes))} · ↓ ${esc(bytes(item.downlink_total_bytes))}</small>`,
        `${badge(item.status || 'unknown')}<small>${esc(item.age_ms == null ? '—' : `${Math.round(finite(item.age_ms) / 100) / 10} с`)} · ${esc(item.interval_ms || 1000)} ms</small>`,
        `<span class="tag">${esc(item.scope || 'user')}</span>${item.gap ? '<span class="badge amber">gap</span>' : ''}${item.warming ? '<span class="badge neutral">warming</span>' : ''}${item.coarse ? '<span class="badge amber">coarse</span>' : ''}`,
        history,
      ];
    });
    const actions = `<button type="button" data-action="export-live" data-format="csv">${icon('download')}CSV</button><button type="button" data-action="export-live" data-format="json">JSON</button>`;
    return pageHead(actions) + `<div class="metrics">${metric('Исходящий сейчас', speed(totalUp), 'Сумма по всем строкам user/node', 'live')}${metric('Входящий сейчас', speed(totalDown), 'Сумма по всем строкам user/node', 'live')}${metric('Потоки user/node', count(all.length), 'Каждая строка — отдельная нода и scope', 'users')}${metric('Coarse inbound', count(coarse), 'Входящие счётчики без атрибуции пользователя', 'clock')}</div>` +
      `<div class="toolbar"><label>Период графика<select id="traffic-period" data-filter="minutes"><option value="15"${String(filters().minutes) === '15' ? ' selected' : ''}>15 минут</option><option value="60"${!filters().minutes || String(filters().minutes) === '60' ? ' selected' : ''}>1 час</option><option value="360"${String(filters().minutes) === '360' ? ' selected' : ''}>6 часов</option><option value="1440"${String(filters().minutes) === '1440' ? ' selected' : ''}>24 часа</option></select></label><small>Поток обновляется через SSE каждую секунду; строка исчезает после окна свежести.</small></div>` + chartPanel() + `<div class="section-gap">${toolbar('Поиск по пользователю, email, ноде или scope')}${panel('Кто и с какой скоростью передаёт данные сейчас', table(['Пользователь', 'Нода', '↑ Upload', '↓ Download', 'Дельты и totals', 'Состояние', 'Scope', 'История'], rows, { title: 'Свежих измерений нет', detail: 'Проверьте heartbeat агента и Xray StatsService. Первый sample помечается warming; gap не превращается в нулевой трафик.', icon: 'live' }), `<small>${count(data.length)} строк · ${state.stream ? 'SSE подключён' : 'polling'}</small>`)}</div>`;
  }

  function agentsPage() {
    const list = state.data.list?.data || [];
    const nodes = state.data.nodes?.data || [];
    const q = String(filters().q || '').toLowerCase();
    const rows = list.filter(item => !q || JSON.stringify(item).toLowerCase().includes(q)).map(item => {
      const node = nodes.find(node => node.id === item.node_id);
      return [`<strong>${esc(node?.name || item.node_id)}</strong><small class="mono">${esc(item.node_id)}</small>`, badge(item.status === 'revoked' ? 'revoked' : item.online ? item.status : 'offline'), esc(item.version || '—'), esc(date(item.last_heartbeat)), `${count(item.memory_mb)} МиБ`, `<div class="table-actions">${actionButton('agent-diagnostics', 'Диагностика', `data-id="${esc(item.node_id)}"`)}${canWrite() && item.status !== 'revoked' ? actionButton('revoke-agent', 'Отозвать', `data-id="${esc(item.node_id)}"`, 'danger') : ''}</div>`];
    });
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="enrollment">${icon('plus')}Подключить агент</button>` : '') + `<div class="notice">Агент отправляет скорость из Xray StatsService и события из access.log. Онлайн означает heartbeat за последние 90 секунд. Ошибки источников видны в диагностике.</div>` + toolbar('Поиск по ноде, версии или ошибке') + panel('Подключённые коллекторы', table(['Нода', 'Состояние', 'Версия', 'Heartbeat', 'Память Go', 'Действия'], rows, { title: 'Агенты ещё не подключены', detail: nodes.length ? 'Нажмите «Подключить агент», создайте одноразовый токен и выполните команду на ноде.' : 'Сначала синхронизируйте ноды с Remnawave, затем создайте токен для выбранной ноды.', icon: 'agents' }), `<small>${count(list.filter(item => item.online && item.status !== 'revoked').length)} онлайн</small>`);
  }
  function configPage() {
    const kind = state.route;
    const list = state.data.list || { data: [], total: 0 };
    const schema = schemas[kind];
    const rows = (list.data || []).map(item => {
      const summary = item.message || '—';
      return [`<strong>${esc(item.name || item.id)}</strong><small class="mono">${esc(item.id)}</small>`, `<span class="wrap-cell">${esc(summary)}</span>`, kind === 'alerts' ? badge(item.severity || 'info') : badge(item.enabled === true ? 'active' : item.enabled === false ? 'disabled' : 'unknown'), `<div class="table-actions">${actionButton('edit-config', canWrite() ? 'Изменить' : 'Открыть', `data-kind="${kind}" data-id="${esc(item.id)}"`)}${canWrite() ? actionButton('delete-config', 'Удалить', `data-kind="${kind}" data-id="${esc(item.id)}"`, 'danger') : ''}</div>`];
    });
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="new-config" data-kind="${kind}">${icon('plus')}Создать</button>` : '') + `<details class="notice"><summary>Поддерживаемые поля и поведение</summary><p>${esc(schema.help)}</p></details>` + toolbar('Поиск по имени, ID или содержимому') + panel(pages[kind][0], table(['Запись', 'Конфигурация', kind === 'alerts' ? 'Уровень' : 'Состояние', 'Действия'], rows, { title: 'Записей пока нет', detail: 'Нажмите «Создать» и заполните форму. Расширенный JSON-редактор доступен в окне настройки.', icon: kind }) + pageFoot(list.total || 0, (list.data || []).length), `<small>${count(list.total)} записей</small>`);
  }
  const incidentLabels = { connection_rate: 'Частота записей соединений', destination_breadth: 'Много IP назначения', port_breadth: 'Много портов одного назначения', dpi_protocol: 'Классификация nDPI', sustained_overload: 'Длительная перегрузка' };
  const reviewLabels = { new: 'Новое', reviewing: 'На разборе', confirmed: 'Подтверждено', false_positive: 'Ложное срабатывание', resolved: 'Закрыто' };
  function incidentJournalLink(item) {
    const start = parseDate(item.first_seen);
    const query = new URLSearchParams({ user: item.user_id, node: item.node_id });
    if (start) {
      const minute = Math.floor(start.getTime() / 60000) * 60000;
      query.set('from', new Date(minute).toISOString());
      query.set('to', new Date(minute + 60000).toISOString());
    }
    return `#/activity?${query}`;
  }
  function incidentsPage() {
    const all = state.data.list?.data || [];
    const f = filters();
    const q = String(f.q || '').toLowerCase();
    const items = all.filter(item => (!f.status || item.status === f.status) && (!q || JSON.stringify(item).toLowerCase().includes(q)));
    const rows = items.map(item => [
      `<strong>${esc(incidentLabels[item.kind] || item.kind)}</strong><small>${item.kind === 'dpi_protocol' ? 'nDPI · метаданные потока' : item.kind === 'sustained_overload' ? 'Интерфейс ноды · несколько минут' : 'Xray access.log · окно 60 сек (UTC)'}</small>${item.truncated ? '<small class="negative">Неполные данные или неоднозначная атрибуция</small>' : ''}`,
      `<a href="${esc(incidentJournalLink(item))}" class="mono">${esc(item.user_id)}</a><small class="mono">Нода: ${esc(item.node_id)}</small>`,
      `<strong>${count(item.value)} / ${count(item.threshold)}</strong><small>${item.target ? esc(item.target) : 'Значение / порог наблюдения'}</small>`,
      `<span class="badge ${item.status === 'new' ? 'amber' : item.status === 'confirmed' ? 'red' : 'neutral'}">${esc(reviewLabels[item.status] || item.status)}</span>${item.note ? `<small class="wrap-cell">${esc(item.note)}</small>` : ''}`,
      `${esc(date(item.last_seen))}<small>С ${esc(date(item.first_seen, true))}</small>`,
      actionButton('review-incident', 'Разобрать', `data-id="${esc(item.id)}"`, 'primary'),
    ]);
    return pageHead() + `<div class="metrics">${metric('Новые', count(all.filter(i => i.status === 'new').length), 'В последних 200 наблюдениях', 'alerts')}${metric('На разборе', count(all.filter(i => i.status === 'reviewing').length), 'Требуют проверки контекста', 'clock')}${metric('Подтверждены', count(all.filter(i => i.status === 'confirmed').length), 'Решение владельца', 'check')}${metric('Ложные сигналы', count(all.filter(i => i.status === 'false_positive').length), 'Для настройки порогов', 'ipintel')}</div>` +
      '<div class="notice">Порог и метка протокола — повод проверить активность. Автоматическое ограничение требует включённой политики, повторных nDPI-подтверждений, точной атрибуции и значительного вклада аккаунта в длительную перегрузку. Команды и отчёты доступны в правилах анализа.</div>' +
      toolbar('Поиск по аккаунту, ноде, признаку или заметке') + `<div class="queue-filters"><label>Состояние<select id="incident-status" data-filter="status"><option value="">Все состояния</option>${Object.entries(reviewLabels).map(([value,label]) => `<option value="${value}" ${f.status === value ? 'selected' : ''}>${label}</option>`).join('')}</select></label><span>Последние 200 наблюдений · хранение 14 дней</span></div>` +
      panel('Очередь разбора', table(['Наблюдение', 'Аккаунт / нода', 'Измерение', 'Решение', 'Обнаружено', 'Действие'], rows, { title: 'Наблюдений по фильтру нет', detail: 'Это не подтверждает отсутствие нарушений. Проверьте подключение агентов и полноту логов Xray.', icon: 'alerts' }));
  }
  function analysisPage() {
    const p = state.data.policy || {};
    const pairs = [['Записей соединений за минуту', p.connections_per_minute], ['Портов одного назначения за минуту', p.ports_per_host_per_minute], ['Разных IP назначения за минуту', p.ips_per_minute]];
    return pageHead(actionButton('edit-analysis', 'Настроить пороги', '', 'primary')) +
      '<div class="notice">Пороги создают наблюдения для разбора. Окна фиксированные минутные; события на границе окна разделяются. Задержанные более чем на 2 минуты события сохраняются в журнале и не создают текущих сигналов.</div>' +
      `<div class="split">${panel('Наблюдения Xray', `<div class="panel-body">${pairs.map(([label,value]) => `<div class="setting-row"><span>${label}</span><strong>${count(value)}</strong></div>`).join('')}<p class="help-line">Считаются уникальные записи лога, не одновременные соединения.</p><div class="setting-row"><span>Перегрузка / восстановление</span><strong>${count(p.overload_percent)}% / ${count(p.recovery_percent)}%</strong></div><div class="setting-row"><span>Последовательных минут перегрузки</span><strong>${count(p.sustained_minutes)}</strong></div></div>`)}${panel('Временный shaper', `<div class="panel-body"><span class="badge ${p.automatic_shaper ? 'red' : 'neutral'}">${p.automatic_shaper ? 'Автоматика включена' : 'Автоматика выключена'}</span><div class="setting-row"><span>Лимит на направление</span><strong>${p.shaper_mbps ? count(p.shaper_mbps) + ' Мбит/с' : 'Не задан'}</strong></div><div class="setting-row"><span>Срок ограничения</span><strong>${p.shaper_minutes ? count(p.shaper_minutes) + ' мин' : 'Не задан'}</strong></div><p>Ограничиваемые протоколы: ${esc((p.restricted_protocols || []).join(', ') || 'Не выбраны')}</p><p>Исключений: ${count((p.allowlist || []).length)}</p><p class="help-line">Требуется подключённый Linux worker и выделенные интерфейсы аккаунтов. Потери, догадки nDPI и общий IP исключают автоматическое решение. TTL снимается на ноде независимо от Core.</p></div>`)}</div>` +
      `<div class="section-gap">${panel('nDPI и длительная перегрузка', '<div class="panel-body"><p>Коллектор передаёт протокол, качество классификации, версию, объёмы и потери. Содержимое трафика не передаётся. Расход и P2P сами по себе не разрешают ограничение.</p><p>Перегрузка учитывает ёмкость интерфейса, полноту минут, потери, CPU, исторический baseline и два окна восстановления. Для ограничения проверяется вклад аккаунта по независимым счётчикам Xray.</p></div>')}</div>` +
      `<div class="section-gap">${panel('Коллекторы и нагрузка нод', table(['Нода', 'Ёмкость / нагрузка', 'Качество', 'Последний отчёт'], (state.data.monitor?.data || []).map(item => [
        `<strong class="mono">${esc(item.node_id)}</strong>${item.overloaded ? '<small class="negative">Длительная перегрузка</small>' : ''}`,
        `${count(item.capacity_mbps)} Мбит/с<small>Последняя минута: ${count(item.window?.Mbps || 0)} Мбит/с · baseline ${count(item.baseline_mbps || 0)}</small>`,
        `<span class="wrap-cell">${esc(item.collector_error || 'Ошибок коллектора нет')}</span><small>${item.window?.Complete ? 'Минутное окно полное' : 'Недостаточно данных окна'} · потоков ${count(item.flows)}</small>`,
        esc(date(item.received_at)),
      ]), { title: 'Коллекторы не подключены', detail: 'Подключите дополнительный Linux worker на нодах.', icon: 'agents' }))}</div>` +
      `<div class="section-gap">${panel('Команды shaper и отчёты нод', table(['Аккаунт / нода', 'Лимит / срок', 'Состояние', 'Основание', 'Действие'], (state.data.shaper?.data || []).map(item => [
        `<strong class="mono">${esc(item.action.user_id)}</strong><small>${esc(item.action.node_id)}</small>`,
        `${count(item.action.mbps)} Мбит/с<small>До ${esc(date(item.action.expires_at))}</small>`,
        `${badge(item.status)}<small>${esc(item.result?.error || '')}</small>`,
        `<span class="wrap-cell">${esc(item.action.reason)}</span>`,
        ['pending','applied','failed'].includes(item.status) ? actionButton('cancel-shaper', 'Снять', `data-id="${esc(item.action.id)}"`, 'danger') : '—',
      ]), { title: 'Команд пока нет', detail: 'Новые команды появятся при выполнении всех условий политики.', icon: 'analysis' }))}</div>`;
  }
  function activityQuery(f = filters(), exportFormat) {
    const query = new URLSearchParams({ limit: exportFormat ? '10000' : '500' });
    for (const key of ['user', 'node', 'domain']) if (f[key]) query.set(key, f[key]);
    for (const key of ['from', 'to']) {
      if (!f[key]) continue;
      const parsed = parseDate(f[key]);
      if (parsed) query.set(key, parsed.toISOString());
    }
    if (exportFormat) query.set('format', exportFormat);
    return query;
  }
  function activityPage() {
    const f = filters();
    const list = state.data.list?.data || [];
    const rows = list.map(item => [esc(date(item.timestamp)), `<strong class="mono">${esc(item.user_id)}</strong><small class="mono">${esc(item.node_id)}</small>`, `<button type="button" class="text-button mono" data-action="lookup-ip" data-ip="${esc(item.src_ip)}">${esc(item.src_ip || '—')}</button>`, `<strong class="mono">${esc(item.dst_host || '—')}</strong><small>${esc(item.network || '')} · :${esc(item.dst_port)}</small>`, esc(item.inbound_tag || '—'), esc(item.outbound_tag || '—'), badge(item.result || 'unknown')]);
    return pageHead(`<button type="button" data-action="export-activity" data-format="csv">${icon('download')}CSV</button><button type="button" data-action="export-activity" data-format="json">${icon('download')}JSON</button>`) +
      `<form id="activity-filters" class="filter-grid"><label>Пользователь (ID)<input id="activity-user" name="user" data-filter="user" value="${esc(f.user)}" placeholder="Все пользователи"></label><label>Нода (ID)<input id="activity-node" name="node" data-filter="node" value="${esc(f.node)}" placeholder="Все ноды"></label><label>Домен / назначение<input id="activity-domain" name="domain" data-filter="domain" value="${esc(f.domain)}" placeholder="example.com"></label><label>С даты (локальное время)<input id="activity-from" name="from" data-filter="from" type="datetime-local" value="${esc(f.from)}"></label><label>По дату (локальное время)<input id="activity-to" name="to" data-filter="to" type="datetime-local" value="${esc(f.to)}"></label><div class="filter-buttons"><button type="submit" class="primary">Применить</button><button type="button" data-action="reset-activity">Сбросить</button>${canWrite() ? '<button type="button" class="danger" data-action="purge-activity">Удалить журнал пользователя</button>' : ''}</div></form>` +
      panel('События соединений', table(['Время', 'Пользователь / нода', 'Источник', 'Назначение', 'Inbound', 'Outbound', 'Результат'], rows, { title: 'Нет событий по выбранным условиям', detail: 'Измените фильтры или проверьте сбор access.log агентом. Для атрибуции журнал Xray должен содержать email пользователя.', icon: 'activity' }), `<small>${count(list.length)} записей · максимум 500</small>`) + `<p class="help-line">Горячий журнал хранится до 14 дней. Экспорт ограничен 10 000 событиями. Домен фильтруется внутри выборки последних событий.</p>`;
  }
  function intelAttribution(sources) {
    return (sources || []).some(source => /db[-_ ]?ip/i.test(typeof source === 'string' ? source : source.database_type || '')) ? '<p class="help-line source-attribution">IP geolocation by <a href="https://db-ip.com/" target="_blank" rel="noopener noreferrer">DB-IP</a></p>' : '';
  }
  function ipintelPage() {
    const result = state.data.result;
    const labels = { ip: 'IP-адрес', asn: 'ASN', as_org: 'Организация', country: 'Страна', region: 'Регион', city: 'Город', network_type: 'Тип сети', confidence: 'Уверенность', timezone: 'Часовой пояс', build_date: 'Сборка источника', sources: 'Источники' };
    const health = state.status?.ipintel || {};
    const sources = Array.isArray(health.sources) ? health.sources : [];
    const sourceRows = sources.map(source => [esc(source.database_type || '—'), esc(date(source.build_date)), source.stale_warning ? `<span class="negative wrap-cell">${esc(source.stale_warning)}</span>` : badge('active')]);
    return pageHead(canWrite() ? `<button type="button" data-action="reload-ipintel">${icon('refresh')}Перезагрузить базы</button>` : '') + `<form id="ip-lookup" class="toolbar"><label class="search-field"><span class="sr-only">IPv4 или IPv6 адрес</span><input id="lookup-input" name="ip" data-filter="ip" value="${esc(filters().ip)}" required placeholder="Введите IPv4 или IPv6" autocomplete="off" spellcheck="false"></label><button type="submit" class="primary">${icon('ipintel')}Проверить</button></form>` + panel('Результат проверки', result ? `<div class="panel-body">${result.stale_warning ? `<div class="notice error">${esc(result.stale_warning)}</div>` : ''}${result.confidence === 'low' ? '<div class="notice">Недостаточно данных для уверенного определения. Неизвестные поля остаются пустыми.</div>' : ''}<div class="key-value">${Object.entries(labels).map(([key, label]) => `<div><small>${label}</small><strong>${esc(key === 'build_date' ? date(result[key]) : Array.isArray(result[key]) ? result[key].join(', ') || 'Нет источников' : result[key] || '—')}</strong></div>`).join('')}</div>${intelAttribution(result.sources)}</div>` : empty('Проверьте IP-адрес', 'Core использует установленные локальные базы. При отсутствии данных результат будет иметь низкую уверенность.', 'ipintel')) + `<div class="section-gap">${panel('Установленные IP-базы', sourceRows.length ? table(['Источник', 'Дата сборки', 'Состояние'], sourceRows) + `<div class="panel-body">${intelAttribution(sources)}</div>` : `<div class="panel-body"><p>MMDB-источники не загружены. Записей префиксов в локальной базе: ${count(health.prefix_records)}.</p></div>`, `<small>${count(health.mmdb_sources ?? sources.length)} MMDB</small>`)}</div>` + '<p class="help-line">Перезагрузка применяет файлы из настроенного каталога IP-данных без перезапуска Core.</p>';
  }
  function backupsPage() {
    const list = state.data.list?.data || [];
    const rows = list.map(item => [`<strong class="mono">${esc(item.name)}</strong>`, esc(date(item.created_at)), bytes(item.size), badge('active').replace('Активен', 'Зашифровано'), canWrite() ? `<button type="button" data-action="download-backup" data-name="${esc(item.name)}">${icon('download')}Скачать</button>` : '<span class="muted">Войдите в аккаунт</span>']);
    return pageHead(canWrite() ? `<button type="button" class="primary" data-action="create-backup">${icon('plus')}Создать копию</button>` : '') + `<div class="notice">Копия содержит согласованный снимок SQLite и шифруется ключом Core. Храните ключ шифрования отдельно: без него восстановление невозможно. </div>` + panel('Снимки базы', table(['Файл', 'Создан', 'Размер', 'Защита', 'Действия'], rows, { title: 'Резервных копий пока нет', detail: 'Создайте первую копию перед важными изменениями конфигурации.', icon: 'backups' }), `<small>${count(list.length)} копий</small>`);
  }
  function auditPage() {
    const q = String(filters().q || '').toLowerCase();
    const list = (state.data.list?.data || []).filter(item => !q || JSON.stringify(item).toLowerCase().includes(q));
    const rows = list.map(item => [esc(date(item.timestamp)), `<span class="mono">#${esc(item.admin_id || '—')}</span>`, `<strong class="mono">${esc(item.action)}</strong>`, `<span class="mono">${esc(item.resource || '—')}</span><small class="mono">${esc(item.resource_id || '')}</small>`, badge(item.status), `<span class="mono">${esc(item.ip || '—')}</span>`]);
    return pageHead() + toolbar('Поиск по действию, ресурсу, IP или администратору') + panel('Последние действия', table(['Время', 'Администратор', 'Действие', 'Ресурс', 'Результат', 'IP'], rows, { title: 'Нет записей по запросу', detail: 'Здесь отображаются последние 200 действий. Попробуйте другой поисковый запрос.', icon: 'audit' }), '<small>Последние 200 записей</small>');
  }
  function settingsPage() {
    const cfg = state.data.settings || {};
    const s = state.data.status || {};
    const groups = [
      ['Core и доступ', [['HTTP listener', cfg.listen], ['Режим', cfg.demo ? 'Demo' : 'Рабочий'], ['Тайм-аут сессии', `${cfg.session_timeout_hours ?? '—'} ч`], ['Учётная запись', 'Владелец — полный доступ'], ['Шифрование активности', cfg.activity_encrypted ? 'Включено' : 'Отключено'], ['Хранение активности', `${cfg.activity_hot_days ?? '—'} дней`]]],
      ['Связь с Remnawave', [['Адрес панели', cfg.panel_url || 'Не настроен'], ['Интервал синхронизации', `${cfg.sync_interval_seconds ?? '—'} сек`], ['Последняя синхронизация', date(s.last_sync)], ['Ошибка синхронизации', s.sync_error || 'Нет']]],
      ['Хранение данных', [['Резервные копии', cfg.backup_path || '—'], ['IP Intelligence', cfg.ipintel_data_path || '—'], ['Uptime', duration(s.uptime_seconds)], ['Событий в журнале', count(s.activity_events)]]],
    ];
    return pageHead() + '<div class="notice">Настройки доступны для просмотра. Изменение параметров выполняется в конфигурации сервиса с перезапуском Core. Секреты и пароли в консоль не передаются.</div>' + `<div class="split">${groups.map(([title, pairs]) => panel(title, `<div class="panel-body">${pairs.map(([label, value]) => `<div class="setting-row"><span>${esc(label)}</span><strong>${esc(value)}</strong></div>`).join('')}</div>`)).join('')}${panel('Диагностика компонентов', `<div class="panel-body"><h3>База данных</h3><pre class="code">${esc(JSON.stringify(s.database || {}, null, 2))}</pre><h3>IP Intelligence</h3><pre class="code">${esc(JSON.stringify(s.ipintel || {}, null, 2))}</pre></div>`)}</div>`;
  }

  function drawChart(canvas, input) {
    const context = canvas.getContext('2d');
    if (!context) return;
    const points = input.map(point => ({ time: parseDate(point.timestamp)?.getTime(), up: Math.max(0, finite(point.uplink_bps ?? point.up)), down: Math.max(0, finite(point.downlink_bps ?? point.down)) })).filter(point => Number.isFinite(point.time)).sort((a, b) => a.time - b.time);
    let dimensions;
    function draw(selected = -1) {
      const rect = canvas.getBoundingClientRect();
      const ratio = Math.min(window.devicePixelRatio || 1, 2);
      const width = Math.max(1, rect.width), height = Math.max(1, rect.height);
      canvas.width = Math.round(width * ratio); canvas.height = Math.round(height * ratio);
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
      context.clearRect(0, 0, width, height);
      const left = Math.min(73, width * 0.23), right = 15, top = 12, bottom = 28;
      const chartW = Math.max(1, width - left - right), chartH = Math.max(1, height - top - bottom);
      const max = Math.max(1, ...points.map(p => Math.max(p.up, p.down))) * 1.12;
      const start = points[0]?.time || Date.now() - 3600000;
      const end = points[points.length - 1]?.time || Date.now();
      const span = Math.max(10000, end - start);
      const x = p => points.length === 1 ? left + chartW / 2 : left + (p.time - start) / span * chartW;
      const y = value => top + chartH * (1 - value / max);
      dimensions = { left, chartW, x };
      context.lineWidth = 1;
      context.font = '10px Segoe UI, sans-serif';
      context.fillStyle = '#718697'; context.textAlign = 'right';
      for (let i = 0; i <= 4; i++) {
        const pos = top + chartH * i / 4;
        context.strokeStyle = '#23303b'; context.beginPath(); context.moveTo(left, pos); context.lineTo(width - right, pos); context.stroke();
        if (points.length) context.fillText(speed(max * (1 - i / 4)).replace('бит/с', '').trim(), left - 10, pos + 3);
      }
      if (!points.length) return;
      context.textAlign = 'left'; context.fillText(date(start, true), left, height - 6);
      context.textAlign = 'right'; context.fillText(date(end, true), width - right, height - 6);
      const paint = (key, color) => {
        const gradient = context.createLinearGradient(0, top, 0, height - bottom);
        gradient.addColorStop(0, `${color}32`); gradient.addColorStop(1, `${color}00`);
        context.beginPath(); context.moveTo(x(points[0]), top + chartH);
        for (const point of points) context.lineTo(x(point), y(point[key]));
        context.lineTo(x(points[points.length - 1]), top + chartH); context.closePath(); context.fillStyle = gradient; context.fill();
        context.beginPath();
        points.forEach((point, index) => { if (!index) context.moveTo(x(point), y(point[key])); else context.lineTo(x(point), y(point[key])); });
        context.strokeStyle = color; context.lineWidth = 2; context.lineJoin = 'round'; context.stroke();
        if (points.length === 1) { context.beginPath(); context.arc(x(points[0]), y(points[0][key]), 3, 0, Math.PI * 2); context.fillStyle = color; context.fill(); }
      };
      paint('down', '#68bddd'); paint('up', '#70e5bd');
      if (selected >= 0 && points[selected]) {
        const point = points[selected];
        context.beginPath(); context.moveTo(x(point), top); context.lineTo(x(point), top + chartH); context.lineWidth = 1; context.strokeStyle = '#7e9b91'; context.setLineDash([3, 3]); context.stroke(); context.setLineDash([]);
        const reading = $('#chart-reading');
        if (reading) reading.textContent = `${date(point.time, true)} · ↑ ${speed(point.up)} · ↓ ${speed(point.down)}`;
      }
    }
    const onMove = event => {
      if (!points.length || !dimensions) return;
      const position = event.clientX - canvas.getBoundingClientRect().left;
      let closest = 0;
      points.forEach((point, index) => { if (Math.abs(dimensions.x(point) - position) < Math.abs(dimensions.x(points[closest]) - position)) closest = index; });
      draw(closest);
    };
    const onLeave = () => { draw(); const reading = $('#chart-reading'); if (reading) reading.textContent = points.length ? `${count(points.length)} точек · шаг 10 секунд` : 'Только реальные данные агентов'; };
    canvas.addEventListener('pointermove', onMove); canvas.addEventListener('pointerleave', onLeave);
    const resize = () => draw();
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(resize) : null;
    if (observer) observer.observe(canvas); else window.addEventListener('resize', resize);
    draw();
    state.chartCleanup = () => { observer?.disconnect(); window.removeEventListener('resize', resize); canvas.removeEventListener('pointermove', onMove); canvas.removeEventListener('pointerleave', onLeave); };
  }

  function openDialog(title, content, submit, onSubmit) {
    closeDialog();
    const dialogRevision = state.dialogRevision;
    modalContent.innerHTML = `<h2 id="dialog-title">${esc(title)}</h2>${content}<div class="notice error modal-error" data-modal-error hidden role="alert"></div><div class="dialog-actions"><button type="button" data-action="close-dialog">${submit ? 'Отмена' : 'Закрыть'}</button>${submit ? `<button type="submit" class="primary">${esc(submit)}</button>` : ''}</div>`;
    modal.setAttribute('aria-labelledby', 'dialog-title');
    modalForm.onsubmit = async event => {
      event.preventDefault();
      if (!onSubmit || modalForm.dataset.busy) return;
      const button = $('button[type="submit"]', modalForm);
      const errorBox = $('[data-modal-error]', modalForm);
      modalForm.dataset.busy = '1';
      if (button) button.disabled = true;
      errorBox.hidden = true;
      try { await onSubmit(new FormData(modalForm)); }
      catch (error) { if (error.status === 401) handleError(error); else if (dialogRevision === state.dialogRevision) { errorBox.textContent = error.message; errorBox.hidden = false; } }
      finally { delete modalForm.dataset.busy; if (button) button.disabled = false; }
    };
    modal.showModal();
  }
  function closeDialog() { state.dialogRevision++; state.configEditor = null; if (modal.open) modal.close(); modalForm.onsubmit = null; delete modalForm.dataset.busy; }
  function confirmAction(title, message, callback, submit = 'Подтвердить') {
    openDialog(title, `<p>${esc(message)}</p>`, submit, async () => { const revision = state.dialogRevision; await callback(); if (revision === state.dialogRevision) closeDialog(); });
  }
  function objectByID(kind, id) {
    const list = kind === 'nodes' && state.route === 'overview' ? state.data.nodes?.data : state.data.list?.data;
    return (list || []).find(item => String(item.id) === id);
  }
  function option(value, label, selected) { return `<option value="${esc(value)}"${String(value) === String(selected) ? ' selected' : ''}>${esc(label)}</option>`; }
  function readConfigJSON() {
    let payload;
    try { payload = JSON.parse($('#config-json').value); } catch (_) { throw new Error('JSON содержит ошибку синтаксиса. Проверьте запятые и кавычки.'); }
    if (!payload || Array.isArray(payload) || typeof payload !== 'object') throw new Error('Конфигурация должна быть JSON-объектом.');
    return payload;
  }
  async function editConfig(kind, id) {
    const existing = id ? objectByID(kind, id) : null;
    if (id && !existing) { toast('Запись больше недоступна. Обновите страницу.', true); return; }
    const schema = schemas[kind];
    const value = existing || { ...schema.sample };
    if (kind === 'alerts' && !existing) value.created_at = new Date().toISOString();
    const shortID = crypto.randomUUID ? crypto.randomUUID().slice(0, 8) : Array.from(crypto.getRandomValues(new Uint8Array(4)), value => value.toString(16).padStart(2, '0')).join('');
    const objectID = id || `alert-${shortID}`;
    const fields = `<label>ID записи<input name="id" id="config-id" maxlength="128" value="${esc(objectID)}" required ${id ? 'readonly' : ''}></label><label class="editor-label">Конфигурация JSON<textarea name="json" id="config-json" spellcheck="false">${esc(JSON.stringify(value, null, 2))}</textarea></label><p class="help-line">${esc(schema.help)}</p>`;
    openDialog(`${id ? 'Конфигурация' : 'Создать'} · ${pages[kind][0]}`, fields, canWrite() ? 'Сохранить' : null, async form => {
      const dialogRevision = state.dialogRevision;
      const objectID = String(form.get('id') || '').trim();
      if (!objectID || objectID.length > 128 || /[/\\?#]/.test(objectID)) throw new Error('Укажите ID до 128 символов без /, \\, ? и #.');
      const payload = readConfigJSON();
      await request(`/${kind}/${encodeURIComponent(objectID)}`, { method: 'PUT', body: payload });
      if (dialogRevision === state.dialogRevision) closeDialog();
      toast('Конфигурация сохранена'); await loadPage();
    });
  }
  function enrollmentDialog() {
    const nodes = state.data.nodes?.data || [];
    if (!nodes.length) { toast('Сначала синхронизируйте ноды с Remnawave.', true); return; }
    openDialog('Подключить агент', `<p>Выберите ноду. Core выдаст одноразовый токен на 15 минут. Повторное подключение этой ноды заменит прежний токен агента.</p><label>Нода<select name="node_id" required>${nodes.map(node => `<option value="${esc(node.id)}">${esc(node.name || node.id)} · ${esc(node.ip_address || node.id)}</option>`).join('')}</select></label>`, 'Создать токен', async form => {
      const revision = state.revision;
      const dialogRevision = state.dialogRevision;
      const result = await request('/agents/enrollment-tokens', { method: 'POST', body: { node_id: String(form.get('node_id')) } });
      if (revision !== state.revision || dialogRevision !== state.dialogRevision) return;
      const quote = value => `'${String(value).replace(/'/g, "'\\''")}'`;
      const installer = `${location.origin}/install-agent.sh`;
      const command = `installer=$(mktemp) && curl -fsSL ${quote(installer)} -o "$installer" && sudo bash "$installer" --core-url ${quote(location.origin)} --node-id ${quote(result.node_id)} --enrollment-token ${quote(result.token)}`;
      const update = 'sudo bash /opt/subgate-agent/install-agent.sh --update';
      openDialog('Установка агента на ноду', `<p>Выполните команду на выбранной Linux-ноде с Docker и Compose v2. Она скачает образ, проверит core, StatsService и access.log, сохранит настройки и зарегистрирует агент. Готовность подтверждается принятым heartbeat. Токен действует до ${esc(date(result.expires_at))}.</p><pre class="code" id="enrollment-command">${esc(command)}</pre><button type="button" data-action="copy" data-target="enrollment-command">Скопировать установку</button><h3>Обновление установленного агента</h3><pre class="code" id="agent-command">${esc(update)}</pre><button type="button" data-action="copy" data-target="agent-command">Скопировать обновление</button><p class="help-line">Установщик сам определяет стандартный каталог access.log и его группу. Для другого расположения добавьте --log-dir /путь/к/каталогу; для нестандартного StatsService — --stats-addr адрес. Конфигурация хранится в /opt/subgate-agent, токен — в Docker volume. После сбоя с сохранёнными настройками: sudo bash /opt/subgate-agent/install-agent.sh --resume. Проверка без перезапуска: --check. Обновление скачивает свежий latest. Для фиксации версии добавьте --image ghcr.io/mrzalupa-lolz/subgate-agent:ВЕРСИЯ.</p>`, null);
    });
  }
  function diagnosticsDialog(id) {
    const agent = (state.data.list?.data || []).find(item => item.node_id === id);
    if (!agent) { toast('Агент не найден.', true); return; }
    const d = agent.diagnostics || {};
    openDialog('Диагностика агента', `<p class="mono">${esc(id)}</p><div class="setting-row"><span>Состояние</span>${badge(agent.status, agent.online)}</div><div class="setting-row"><span>Heartbeat</span><strong>${esc(date(agent.last_heartbeat))}</strong></div><div class="setting-row"><span>Uptime</span><strong>${esc(duration(d.uptime_seconds))}</strong></div><div class="setting-row"><span>Память Go</span><strong>${count(agent.memory_mb)} МиБ</strong></div><div class="setting-row"><span>Потерянные события</span><strong>${count(d.dropped_events)}</strong></div><h3>StatsService</h3><div class="notice ${d.stats_error ? 'error' : ''}">${esc(d.stats_error || (agent.diagnostics ? 'Ошибок в последнем heartbeat нет.' : 'Heartbeat с диагностикой ещё не получен.'))}</div><h3>Журнал Xray</h3><div class="notice ${d.logs_error ? 'error' : ''}">${esc(d.logs_error || (agent.diagnostics ? 'Ошибок в последнем heartbeat нет.' : 'Нет данных.'))}</div><p>Проверьте локальный адрес StatsService, доступность access.log и наличие пользовательского email в строках лога.</p>`, null);
  }
  async function download(path, filename) {
    const response = await request(path, { raw: true });
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url; anchor.download = filename; document.body.append(anchor); anchor.click(); anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    toast('Файл подготовлен к скачиванию');
  }
  async function lookupIP(ip) {
    if (!ip) return;
    const revision = state.revision;
    const result = await request(`/ipintel/lookup?${new URLSearchParams({ ip })}`);
    if (revision !== state.revision) return;
    if (state.route === 'ipintel') { state.data.result = result; filters().ip = ip; renderPage(); }
    else openDialog('IP Intelligence', `<p>${esc(ip)}</p>${result.stale_warning ? `<div class="notice error">${esc(result.stale_warning)}</div>` : ''}<pre class="code">${esc(JSON.stringify(result, null, 2))}</pre>${intelAttribution(result.sources)}`, null);
  }
  async function showUserTraffic(user) {
    const revision = state.revision;
    const result = await request(`/users/${encodeURIComponent(user)}/traffic?minutes=1440`);
    if (revision !== state.revision) return;
    const rows = (result.data || []).map(item => [
      esc(date(item.timestamp, true)), `<span class="mono">${esc(item.node_id)}</span>`, `<span class="tag">${esc(item.scope || 'user')}</span>`,
      `<strong class="positive">↑ ${esc(speed(item.uplink_bps))}</strong><small>avg ${esc(speed(item.uplink_avg_bps))} · peak ${esc(speed(item.uplink_peak_bps))} · p95 ${esc(speed(item.uplink_p95_bps))}</small>`,
      `<strong>↓ ${esc(speed(item.downlink_bps))}</strong><small>avg ${esc(speed(item.downlink_avg_bps))} · peak ${esc(speed(item.downlink_peak_bps))} · p95 ${esc(speed(item.downlink_p95_bps))}</small>`,
      `<small>↑ ${esc(bytes(item.uplink_bytes))} · ↓ ${esc(bytes(item.downlink_bytes))}</small><small>${count(item.samples)} samples</small>`,
    ]);
    openDialog(`История трафика · ${user}`, `<p>10-секундные интервалы за последние 24 часа. Строки разделены по ноде и scope; coarse inbound не смешивается с пользовательскими данными.</p>${table(['Время', 'Нода', 'Scope', 'Upload', 'Download', 'Bytes'], rows, { title: 'Истории пока нет', detail: 'Для этого пользователя Core ещё не получил samples.', icon: 'live' })}`, null);
  }
  function validActivityRange() {
    const f = filters();
    if (f.from && f.to && parseDate(f.from)?.getTime() > parseDate(f.to)?.getTime()) { toast('Дата начала должна предшествовать дате окончания.', true); return false; }
    return true;
  }
  async function doAction(button) {
    const { action, kind, id, operation } = button.dataset;
    if (action === 'menu') {
      const open = $('#sidebar').classList.toggle('open');
      button.setAttribute('aria-expanded', String(open)); return;
    }
    if (action === 'close-dialog') { closeDialog(); return; }
    if (action === 'refresh') { await loadPage(); return; }
    if (action === 'export-live') { await download(`/live/export?format=${encodeURIComponent(button.dataset.format || 'json')}`, `subgate-live.${button.dataset.format || 'json'}`); return; }
    if (action === 'user-traffic') { await showUserTraffic(button.dataset.user); return; }
    if (action === 'logout') {
      await request('/auth/logout', { method: 'POST' }); state.user = null; state.status = null; state.data = {}; stopPage(); renderLogin(); return;
    }
    if (action === 'previous' || action === 'next') { filters().offset = Math.max(0, finite(filters().offset) + (action === 'next' ? 50 : -50)); await loadPage(); return; }
    if (action === 'sync') { await request('/panel/sync', { method: 'POST' }); toast('Синхронизация завершена'); await loadPage(); return; }
    if (action === 'details') {
      const revision = state.revision;
      const result = await request(`/${kind}/${encodeURIComponent(id)}`);
      if (revision === state.revision) openDialog(kind === 'nodes' ? 'Данные ноды' : 'Данные пользователя', `<pre class="code">${esc(JSON.stringify(result, null, 2))}</pre>`, null);
      return;
    }
    if (action === 'resource-action') {
      const labels = { enable: 'Включить', disable: 'Отключить', restart: 'Перезапустить', 'reset-traffic': 'Сбросить трафик' };
      const apply = async () => { await request(`/${kind}/${encodeURIComponent(id)}/actions/${operation}`, { method: 'POST' }); toast('Операция выполнена'); await loadPage(); };
      if (operation === 'enable') await apply();
      else confirmAction(labels[operation], `${labels[operation]} для ${id}? ${operation === 'reset-traffic' ? 'Счётчик трафика будет обнулён в Remnawave.' : operation === 'restart' ? 'Активные соединения на ноде могут прерваться.' : 'Доступ будет отключён в Remnawave.'}`, apply, labels[operation]);
      return;
    }
    if (action === 'review-incident') {
      const item = (state.data.list?.data || []).find(item => item.id === id);
      if (!item) return;
      openDialog('Разбор наблюдения', `<p>${esc(incidentLabels[item.kind] || item.kind)}: ${count(item.value)} при пороге ${count(item.threshold)}. Подтверждение фиксирует ваше решение; ограничений не запускает.</p><p><a href="${esc(incidentJournalLink(item))}">Открыть журнал в этом окне</a></p><label>Решение<select name="status">${Object.entries(reviewLabels).filter(([value]) => value !== 'new').map(([value,label]) => `<option value="${value}" ${item.status === value ? 'selected' : ''}>${label}</option>`).join('')}</select></label><label class="section-gap">Основания / заметка<textarea name="note" maxlength="2000" class="review-note">${esc(item.note || '')}</textarea></label>`, 'Сохранить решение', async form => {
        const data = form;
        await request(`/incidents/${encodeURIComponent(id)}/review`, { method: 'PUT', body: { status: data.get('status'), note: data.get('note') } });
        closeDialog(); toast('Решение сохранено'); await loadPage();
      }); return;
    }
    if (action === 'edit-analysis') {
      const p = state.data.policy || {};
      const fields = [['connections_per_minute','Записей соединений / мин',100,8000],['ports_per_host_per_minute','Портов одного назначения / мин',16,1024],['ips_per_minute','Разных IP назначения / мин',32,4096],['overload_percent','Перегрузка, % ёмкости',50,100],['recovery_percent','Восстановление, % ёмкости',10,99],['sustained_minutes','Минут перегрузки подряд',2,60],['shaper_mbps','Лимит на направление, Мбит/с',0,100000],['shaper_minutes','Срок ограничения, мин',0,1440]];
      openDialog('Анализ и временные ограничения', '<p>Автоматика требует лимит, срок и явно выбранные протоколы. Пустые значения оставляют ограничение выключенным.</p><div class="form-grid">' + fields.map(([key,label,min,max]) => `<label>${esc(label)}<input name="${key}" type="number" min="${min}" max="${max}" step="1" ${min ? 'required' : ''} value="${esc(p[key] || '')}"></label>`).join('') + `</div><label class="section-gap">Автоматический shaper<select name="automatic_shaper"><option value="false">Выключен</option><option value="true" ${p.automatic_shaper ? 'selected' : ''}>Включён</option></select></label><label class="section-gap">Ограничиваемые протоколы nDPI (точные имена, по одному на строку)<textarea name="restricted_protocols">${esc((p.restricted_protocols || []).join('\n'))}</textarea></label><label class="section-gap">Исключения (UUID аккаунтов, по одному на строку)<textarea name="allowlist">${esc((p.allowlist || []).join('\n'))}</textarea></label>`, 'Сохранить настройки', async form => {
        const data = form;
        const body = Object.fromEntries(fields.map(([key]) => [key, Number(data.get(key) || 0)]));
        body.automatic_shaper = data.get('automatic_shaper') === 'true';
        body.restricted_protocols = String(data.get('restricted_protocols') || '').split(/\r?\n/).map(s => s.trim()).filter(Boolean);
        body.allowlist = String(data.get('allowlist') || '').split(/\r?\n/).map(s => s.trim()).filter(Boolean);
        await request('/analysis/policy', { method: 'PUT', body });
        closeDialog(); toast('Настройки сохранены'); await loadPage();
      }); return;
    }
    if (action === 'cancel-shaper') {
      await request(`/analysis/shaper/${encodeURIComponent(id)}`, { method: 'DELETE' });
      toast('Снятие запрошено. Нода подтвердит удаление ограничения.'); await loadPage(); return;
    }
    if (action === 'new-config' || action === 'edit-config') { await editConfig(kind, id); return; }
    if (action === 'delete-config') { confirmAction('Удалить запись', `Удалить ${id} из реестра ${pages[kind][0]}?`, async () => { await request(`/${kind}/${encodeURIComponent(id)}`, { method: 'DELETE' }); toast('Запись удалена'); await loadPage(); }, 'Удалить'); return; }
    if (action === 'enrollment') { enrollmentDialog(); return; }
    if (action === 'agent-diagnostics') { diagnosticsDialog(id); return; }
    if (action === 'revoke-agent') { confirmAction('Отозвать доступ агента', `Агент ${id} перестанет отправлять heartbeat и телеметрию. Для подключения понадобится новый токен.`, async () => { await request(`/agents/${encodeURIComponent(id)}`, { method: 'DELETE' }); toast('Токен агента отозван'); await loadPage(); }, 'Отозвать'); return; }
    if (action === 'copy') {
      const element = document.getElementById(button.dataset.target);
      if (!element) return;
      const value = element.value ?? element.textContent;
      if (navigator.clipboard?.writeText) { await navigator.clipboard.writeText(value); toast('Скопировано'); }
      else {
        const range = document.createRange(); range.selectNodeContents(element); const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
        toast('Команда выделена. Нажмите Ctrl+C или выберите «Копировать».');
      }
      return;
    }
    if (action === 'lookup-ip') { await lookupIP(button.dataset.ip); return; }
    if (action === 'reload-ipintel') {
      const result = await request('/ipintel/reload', { method: 'POST' });
      if (state.status) state.status.ipintel = result.health;
      toast('IP-базы перезагружены');
      if (state.route === 'ipintel' && filters().ip) await lookupIP(filters().ip); else await loadPage();
      return;
    }
    if (action === 'reset-activity') { state.filters.activity = { q: '', offset: 0 }; await loadPage(); return; }
    if (action === 'export-activity') { if (validActivityRange()) await download(`/activity/export?${activityQuery(filters(), button.dataset.format)}`, `subgate-activity.${button.dataset.format}`); return; }
    if (action === 'purge-activity') {
      const user = String(filters().user || '').trim();
      if (!user) { toast('Укажите ID пользователя в фильтре перед удалением.', true); $('#activity-user')?.focus(); return; }
      confirmAction('Удалить журнал пользователя', `Все сохранённые события пользователя ${user} будут удалены, включая записи вне выбранного диапазона. Это действие невозможно отменить.`, async () => { await request(`/activity/${encodeURIComponent(user)}`, { method: 'DELETE' }); toast('Журнал пользователя удалён'); await loadPage(); }, 'Удалить события'); return;
    }
    if (action === 'create-backup') { const result = await request('/backups', { method: 'POST' }); toast(`Копия создана: ${result.name}`); await loadPage(); return; }
    if (action === 'download-backup') { await download(`/backups/${encodeURIComponent(button.dataset.name)}`, button.dataset.name); }
  }

  document.addEventListener('click', async event => {
    if (event.target.closest('.skip-link')) { event.preventDefault(); $('#main-content')?.focus(); return; }
    const button = event.target.closest('button[data-action]');
    if (!button || button.disabled) return;
    button.disabled = true;
    try { await doAction(button); } catch (error) { handleError(error); }
    finally { if (button.isConnected) button.disabled = false; }
  });
  document.addEventListener('input', event => {
    const field = event.target.closest('[data-filter]');
    if (!field || !state.user) return;
    filters()[field.dataset.filter] = field.value;
    if (field.dataset.filter !== 'q') return;
    filters().offset = 0;
    clearTimeout(state.searchTimer);
    if (['live', 'agents', 'audit', 'incidents'].includes(state.route)) { renderPage(); return; }
    state.searchTimer = setTimeout(() => loadPage(), 300);
  });
  document.addEventListener('change', event => {
    if (event.target.dataset.filter === 'status') { renderPage(); return; }
    if (event.target.dataset.filter === 'minutes') { filters().minutes = event.target.value; loadPage(); }
  });
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && !modal.open) { $('#sidebar')?.classList.remove('open'); $('[data-action="menu"]')?.setAttribute('aria-expanded', 'false'); }
  });
  document.addEventListener('submit', async event => {
    const form = event.target;
    if (!['activity-filters', 'ip-lookup'].includes(form.id)) return;
    event.preventDefault();
    try {
      if (form.id === 'activity-filters') { if (validActivityRange()) await loadPage(); }
      else {
        const button = $('button[type="submit"]', form); button.disabled = true;
        try { await lookupIP(String(new FormData(form).get('ip') || '').trim()); }
        finally { if (button.isConnected) button.disabled = false; }
      }
    } catch (error) { handleError(error); }
  });
  modal.addEventListener('click', event => { if (event.target === modal) { const rect = modal.getBoundingClientRect(); if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) closeDialog(); } });
  modal.addEventListener('cancel', () => { closeDialog(); });
  window.addEventListener('hashchange', navigate);
  document.addEventListener('visibilitychange', () => { if (!document.hidden && state.user && !state.loading && !modal.open) loadPage(true); });

  function renderLogin(message = '') {
    document.title = 'Вход · SubGate';
    app.innerHTML = `<main class="login"><section class="login-story"><div class="brand"><span class="logo-mark">S</span>SubGate</div><div class="eyebrow">Infrastructure operations</div><h1>Ваша инфраструктура.<br><span>Полная картина.</span></h1><p>Состояние нод, телеметрия Xray и объяснимые наблюдения для ежедневного разбора активности.</p><small>Remnawave · Xray · SubGate Core</small></section><section class="login-form-area"><form id="login-form"><h2>Войти в консоль</h2><p>Используйте аккаунт администратора SubGate.</p>${message ? `<div class="notice error" role="alert">${esc(message)}</div>` : ''}<label>Имя пользователя<input name="username" id="login-username" autocomplete="username" maxlength="128" required autofocus></label><label>Пароль<input name="password" type="password" autocomplete="current-password" maxlength="1024" required></label><div id="login-error" class="notice error" role="alert" hidden></div><button type="submit" class="primary">Войти ${icon('arrow')}</button><div class="demo-hint" id="demo-login-hint" hidden>Demo: admin / subgate-demo</div></form></section></main>`;
    $('#login-form').addEventListener('submit', async event => {
      event.preventDefault();
      const form = event.currentTarget;
      const button = $('button[type="submit"]', form);
      const errorBox = $('#login-error');
      errorBox.hidden = true; button.disabled = true; button.textContent = 'Вход…';
      try {
        const fields = new FormData(form);
        state.user = principal(await request('/auth/login', { method: 'POST', body: { username: String(fields.get('username')).trim(), password: String(fields.get('password')) } }));
        state.status = await request('/system/status');
        renderShell(); await navigate();
      } catch (error) {
        if (form.isConnected) { errorBox.textContent = error.status === 401 ? 'Неверное имя пользователя или пароль.' : error.message; errorBox.hidden = false; }
      } finally { if (button.isConnected) { button.disabled = false; button.innerHTML = `Войти ${icon('arrow')}`; } }
    });
    fetch('/version', { credentials: 'same-origin', cache: 'no-store' }).then(response => response.ok ? response.json() : {}).then(version => { const hint = $('#demo-login-hint'); if (hint) hint.hidden = !version.demo; }).catch(() => {});
  }
  async function start() {
    try {
      state.user = principal(await request('/auth/me'));
      state.status = await request('/system/status');
      renderShell(); await navigate();
    } catch (error) {
      state.user = null;
      renderLogin(error.status === 401 ? '' : error.message);
    }
  }
  start();
})();
