const { defineConfig } = require('vitest/config')

let reporterOptions = {}
if (process.env.CI) {
  reporterOptions = {
    reporters: [
      'default',
      [
        'junit',
        {
          classnameTemplate: `Acceptance tests.{filename}`,
        },
      ],
    ],
    outputFile: 'reports/junit-vitest-acceptance.xml',
  }
}
module.exports = defineConfig({
  test: {
    include: ['test/acceptance/js/**/*.test.{js,ts}'],
    // Runs the migrations and starts the service once for the whole run;
    // per-file setup would do both twice, concurrently. See globalSetup.ts.
    globalSetup: ['./test/acceptance/js/globalSetup.ts'],
    isolate: false,
    ...reporterOptions,
  },
})
