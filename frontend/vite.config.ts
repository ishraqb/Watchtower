import adapterStatic from '@sveltejs/adapter-static';
import adapterVercel from '@sveltejs/adapter-vercel';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

// Pick the adapter based on where we're building.
// Vercel sets process.env.VERCEL=1 during its builds, so deploying there uses
// the Vercel adapter automatically. Everywhere else (local, Cloudflare Pages,
// Netlify) we emit plain static files via adapter-static. The app is a
// client-side SPA either way - see src/routes/+layout.ts.
const adapter = process.env.VERCEL ? adapterVercel() : adapterStatic();

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			adapter
		})
	]
});
