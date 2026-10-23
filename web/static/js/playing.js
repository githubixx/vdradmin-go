(() => {
    const form = document.getElementById('playing-form');
    const main = document.querySelector('.page-playing');
    const grid = main.querySelector('.playing-groups');
    const groups = Array.from(grid.querySelectorAll('.playing-group'));
    const channel = document.getElementById('channel-jump');
    const guide = main.classList.contains('playing-guide');
    const pager = main.querySelector('.playing-pager');
    const previous = document.getElementById('guide-previous');
    const next = document.getElementById('guide-next');
    const slots = Array.from(grid.querySelectorAll('.guide-slot'));
    let selected = 0;
    let pageSize = 1;
    let syncing = false;
    let measuredWidth = -1;

    const anchor = group => group.querySelector('.channels-day').id;
    const indexFor = value => groups.findIndex(group => anchor(group) === value);
    const focusChannel = () => groups[selected]?.querySelector('.channels-day').focus();

    function syncDayLinks() {
        const currentChannel = new URL(location.href).searchParams.get('channel-jump');
        main.querySelectorAll('[data-playing-day]').forEach(link => {
            const url = new URL(link.href);
            if (currentChannel) url.searchParams.set('channel-jump', currentChannel);
            else url.searchParams.delete('channel-jump');
            link.href = url.href;
        });
    }

    function render() {
        if (!guide || groups.length === 0) return;
        pageSize = getComputedStyle(grid).gridTemplateColumns.split(' ').length;
        const start = Math.floor(selected / pageSize) * pageSize;
        groups.forEach((group, index) => {
            group.hidden = index < start || index >= start + pageSize;
        });
        pager.hidden = false;
        previous.disabled = start === 0;
        next.disabled = start + pageSize >= groups.length;
        document.getElementById('guide-range').textContent =
            `Channels ${start + 1}\u2013${Math.min(start + pageSize, groups.length)} of ${groups.length}`;
    }

    function measureTimeline() {
        if (!guide || groups.length === 0) return;
        main.classList.remove('guide-timeline-ready');
        main.classList.add('guide-measuring');
        grid.style.removeProperty('--guide-heading-height');
        groups.forEach(group => { group.hidden = false; });
        slots.forEach(slot => {
            slot.style.setProperty('--guide-slot-width', `${100 / Number(slot.dataset.guideLanes)}%`);
            slot.style.setProperty('--guide-slot-left', `${100 * Number(slot.dataset.guideLane) / Number(slot.dataset.guideLanes)}%`);
        });
        const duration = Number(grid.dataset.guideDuration);
        const intervals = slots.map(slot => ({
            slot,
            start: Number(slot.dataset.guideOffset),
            stop: Number(slot.dataset.guideOffset) + Number(slot.dataset.guideDuration),
            height: Math.ceil(slot.getBoundingClientRect().height + 2),
        }));
        const times = Array.from(new Set([0, duration, ...intervals.flatMap(interval => [interval.start, interval.stop])]))
            .sort((a, b) => a - b);
        const endingAt = new Map(times.map(time => [time, []]));
        intervals.forEach(interval => endingAt.get(interval.stop).push(interval));
        const positions = new Map([[0, 0]]);
        // Each card constrains only its own interval. A forward longest-path pass
        // finds the smallest shared layout without magnifying the whole day.
        for (let i = 1; i < times.length; i++) {
            const time = times[i];
            let position = positions.get(times[i - 1]) + (time - times[i - 1]) / 60;
            for (const interval of endingAt.get(time)) {
                position = Math.max(position, positions.get(interval.start) + interval.height);
            }
            positions.set(time, Math.ceil(position));
        }
        intervals.forEach(({ slot, start, stop }) => {
            slot.style.setProperty('--guide-slot-top', `${positions.get(start)}px`);
            slot.style.setProperty('--guide-slot-height', `${positions.get(stop) - positions.get(start)}px`);
        });
        const headingHeight = groups.reduce((maximum, group) => Math.max(maximum,
            group.querySelector('.channels-day').getBoundingClientRect().height), 0);
        grid.style.setProperty('--guide-heading-height', `${headingHeight}px`);
        grid.style.setProperty('--guide-timeline-height', `${positions.get(duration)}px`);
        main.classList.remove('guide-measuring');
        main.classList.add('guide-timeline-ready');
        measuredWidth = grid.getBoundingClientRect().width;
        render();
    }

    function restore() {
        const url = new URL(location.href);
        const hashIndex = indexFor(url.hash.slice(1));
        const queryIndex = indexFor(url.searchParams.get('channel-jump'));
        selected = Math.max(0, hashIndex >= 0 ? hashIndex : queryIndex);
        syncing = true;
        channel.value = hashIndex >= 0 ? anchor(groups[selected]) : url.searchParams.get('channel-jump') || '';
        channel.dispatchEvent(new Event('change', { bubbles: true }));
        syncing = false;
        render();
        syncDayLinks();
        if (document.activeElement.closest('.playing-group')?.hidden) focusChannel();
    }

    function navigate(index) {
        selected = index;
        const url = new URL(location.href);
        const value = anchor(groups[selected]);
        url.searchParams.set('channel-jump', value);
        url.hash = value;
        history.pushState(null, '', url);
        restore();
        focusChannel();
    }

    channel.addEventListener('change', () => {
        if (syncing) return;
        const index = indexFor(channel.value);
        if (index >= 0) navigate(index);
        else if (!channel.value) {
            const url = new URL(location.href);
            url.searchParams.delete('channel-jump');
            url.hash = '';
            history.pushState(null, '', url);
            restore();
        }
    });
    previous.addEventListener('click', () => navigate(Math.max(0, Math.floor(selected / pageSize) * pageSize - pageSize)));
    next.addEventListener('click', () => navigate(Math.floor(selected / pageSize) * pageSize + pageSize));
    ['playing-start', 'playing-end', 'playing-view'].forEach(id => {
        document.getElementById(id).addEventListener('change', () => form.requestSubmit());
    });
    window.addEventListener('popstate', restore);
    window.addEventListener('hashchange', restore);
    if (guide) {
        new ResizeObserver(() => {
            const focused = groups.indexOf(document.activeElement.closest('.playing-group'));
            if (focused >= 0) selected = focused;
            if (Math.abs(grid.getBoundingClientRect().width - measuredWidth) > 0.5) measureTimeline();
            render();
        }).observe(grid);
        document.fonts.ready.then(measureTimeline);
    }
    restore();
    measureTimeline();
    if (indexFor(location.hash.slice(1)) >= 0) focusChannel();
})();
