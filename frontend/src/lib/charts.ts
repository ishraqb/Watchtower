import {
	createChart,
	LineSeries,
	type IChartApi,
	type ISeriesApi,
	type UTCTimestamp
} from 'lightweight-charts';

export interface SymbolChart {
	chart: IChartApi;
	series: ISeriesApi<'Line'>;
	lastTime: number;
}

const chartOptions = {
	layout: {
		background: { color: 'transparent' },
		textColor: '#cbd5e1'
	},
	grid: {
		vertLines: { color: 'rgba(148, 163, 184, 0.1)' },
		horzLines: { color: 'rgba(148, 163, 184, 0.1)' }
	},
	rightPriceScale: { borderColor: 'rgba(148, 163, 184, 0.2)' },
	timeScale: {
		borderColor: 'rgba(148, 163, 184, 0.2)',
		timeVisible: true,
		secondsVisible: true
	},
	autoSize: true
};

/** Creates a line chart bound to a container element. */
export function createSymbolChart(container: HTMLElement): SymbolChart {
	const chart = createChart(container, chartOptions);
	const series = chart.addSeries(LineSeries, {
		color: '#38bdf8',
		lineWidth: 2,
		priceLineVisible: true,
		lastValueVisible: true
	});
	return { chart, series, lastTime: 0 };
}

/**
 * Pushes a price point onto the series. Lightweight Charts requires strictly
 * increasing timestamps, so duplicate-second ticks are nudged forward by 1s.
 */
export function pushPrice(sc: SymbolChart, epochMillis: number, price: number): void {
	let time = Math.floor(epochMillis / 1000);
	if (time <= sc.lastTime) {
		time = sc.lastTime + 1;
	}
	sc.lastTime = time;
	sc.series.update({ time: time as UTCTimestamp, value: price });
}

/**
 * Replaces the entire series with a historical set of (unix-second, price)
 * points. Used when switching the time-range tab. Colors the line green/red
 * based on whether the series ends above or below where it started.
 */
export function setSeriesData(
	sc: SymbolChart,
	points: { time: number; value: number }[]
): void {
	const data = points
		.filter((p) => Number.isFinite(p.time) && Number.isFinite(p.value))
		.map((p) => ({ time: p.time as UTCTimestamp, value: p.value }));

	if (data.length > 0) {
		const up = data[data.length - 1].value >= data[0].value;
		sc.series.applyOptions({ color: up ? '#34d399' : '#f87171' });
		sc.lastTime = data[data.length - 1].time as number;
	}

	sc.series.setData(data);
	sc.chart.timeScale().fitContent();
}
