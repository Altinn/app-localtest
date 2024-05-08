import { browser } from 'k6/experimental/browser';
import { check, sleep } from 'k6';

// This file serves as a sample loadtest of an app using k6
// To create your own, `cp loadtest.sample.js loadtest.js` and modify it to fit your needs
// To run the loadtest, see the 'k6' Makefile target as an example

// Configuration for k6
// In this case, the loadtester will spin up 4 virtual users, each running the test once concurrently
export const options = {
  scenarios: {
    ui: {
      executor: 'per-vu-iterations', // Doc: https://k6.io/docs/using-k6/scenarios/executors/per-vu-iterations/
      vus: 4,                        // Number of "virtual users"
      iterations: 1,                 // Number of iterations/executions per virtual user
      options: {
        browser: {
          type: 'chromium',
        },
      },
    },
  },
}

// The actual test/execution
// The flow below is loosely based on an app developed
// as part of the Altinn Studio intro course:
// https://docs.altinn.studio/app/app-dev-course/
export default async function () {
  const page = browser.newPage();

  try {
    // Open localtest, wait for load
    const res = await page.goto('http://local.altinn.cloud/');
    check(res, {
      'status is 200': res => res.status() === 200,
    });
    page.waitForLoadState('domcontentloaded');
    sleep(1);

    // Click the login button
    await page.locator('button.btn-primary').click();
    await page.waitForNavigation();
    sleep(1);

    // Click the app/form start button
    await page.locator('#instantiation-button').click();
    await page.waitForNavigation();
    sleep(1);

    // Keep track of data that should be filled in for this test
    const data = {
        address: 'Skuiveien 100',
        postNr: '1337',
    };
    const fields = {
        address: page.locator('#Input-Gateadresse'),
        postNr: page.locator('#Input-Postnr'),
        postSted: page.locator('#Input-Poststed'),
    };

    // Fill in the address field
    fields.address.type(data.address);
    check(fields.address, { 'address is filled in': field => field.inputValue() === data.address, });
    sleep(1);

    // Fill in the Post Nr field
    fields.postNr.type(data.postNr);
    check(fields.postNr, { 'postNr is filled in': field => field.inputValue() === data.postNr, });

    // Poststed is being filled in serverside, so let's wait for that
    await page.waitForFunction('document.querySelector("#Input-Poststed").value === "SANDVIKA"');

    // Go to the next page of the form, twice
    await page.locator('div[data-testid="NavigationButtons"] > div:first-of-type > button').click();
    sleep(1);
    await page.locator('div[data-testid="NavigationButtons"] > div:first-of-type > button').click();
    sleep(1);

    // Try to submit the form, make sure there is no validation error box
    await page.locator('#Button-SendInn').click();
    sleep(1);
    check(page.locator('div[data-testid="ErrorReport"]'), {
        'validation error box does not exist': errorBox => !errorBox.isVisible(),
    });

    // Confirm the submission
    await page.locator('#confirm-button').click();
    sleep(1);
  } finally {
    page.close();
  }
}
