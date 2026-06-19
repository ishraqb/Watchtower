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
