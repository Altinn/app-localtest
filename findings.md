# Loadtest findings

Results are specific to my machine.
Loadtest runs the PDF generation call directly to PDF containers for a prebaked instance containing 2 subform elements and some other data.
All containers are configured with 4 set as max concurrency.
Loadtest settings: 
* 5 minutes duration
* 3 active connections 
* 3 pipelined requests 
* 3 rps target 
(`--duration 300 -c 3 -p 3 -R 3`, using Autocannon for HDR histograms) 

* Old browserless
  * 3072 MB image size
  * 773ms average startup time (time to first 200 health probe)
  * 486 MB memory usage after start
  * 1024 MB memory usage after loadtest
  * Completed 508 requests
  * 1903.07 ms average
  * 4624 ms 99th
  * 5833 ms 99.999th
  * 1233.75 ms stdev

* Go PDF (baseline - same browser version, but only clear cookies)
  * 884 MB image size
  * 230ms average startup time (time to first 200 health probe)
  * 434 MB memory usage after start
  * 1024 MB memory usage after loadtest
  * Completed 781 requests
  * 1155.83 ms average
  * 2296 ms 99th
  * 2869 ms 99.999th
  * 668.1 ms stdev
  * 3 errors (3 timeouts)

* Rust PDF (baseline - same browser version, but only clear cookies)
  * 889 MB image size
  * 232ms average startup time (time to first 200 health probe)
  * 627 MB memory usage after start
  * 972 MB memory usage after loadtest
  * Completed 621 requests
  * 1475.35 ms average
  * 2924 ms 99th
  * 3695 ms 99.999th
  * 852.57 ms stdev

* Go PDF (clear all origin storage, considered safe?)
  * 360 MB image size
  * 231ms average startup time (time to first 200 health probe)
  * 205 MB memory usage after start
  * 1024 MB memory usage after loadtest
  * Completed 907 requests
  * 620.72 ms average
  * 1748 ms 99th
  * 2055 ms 99.999th
  * 407.9 ms stdev

## Stats

Relative performance compared to Old browserless (baseline):

**Go PDF (baseline)**
* Image size: -71% (884 MB vs 3072 MB)
* Startup time: -70% (230ms vs 773ms)
* Memory after start: -11% (434 MB vs 486 MB)
* Requests completed: +54% (781 vs 508)
* Average response time: -39% (1156ms vs 1903ms)
* 99th percentile: -50% (2296ms vs 4624ms)

**Rust PDF (baseline)**
* Image size: -71% (889 MB vs 3072 MB)  
* Startup time: -70% (232ms vs 773ms)
* Memory after start: +29% (627 MB vs 486 MB)
* Requests completed: +22% (621 vs 508)
* Average response time: -22% (1475ms vs 1903ms)
* 99th percentile: -37% (2924ms vs 4624ms)

**Go PDF (clear all storage)**
* Image size: -88% (360 MB vs 3072 MB)
* Startup time: -70% (231ms vs 773ms)
* Memory after start: -58% (205 MB vs 486 MB)
* Requests completed: +79% (907 vs 508)
* Average response time: -67% (621ms vs 1903ms)
* 99th percentile: -62% (1748ms vs 4624ms)

## Conclusion

* Alternatives
  * chromedp /w Go
    * Most efficient apparantly
    * Relatively popular/well maintained (not completely onpar with Puppeteer)
    * Keeps up with puppeteer development
    * Another go service
    * Best k8s client for if/when we want to hijack autoscaling
    * Great HTTP libraries for if/when we want to proxy to centralized pool of PDF workers
    * Native code, fast startup, lightweight
  * Puppeteer service
    * Least efficient
    * Gold standard in terms of implementation (CDP etc)
    * JS on the backend
    * k8s, HTTP libraries unknown quality?
    * JITet, includes runtime, etc etc
  * PuppeteerSharp?
    * Not tested, .NET historically a little bloated
    * K8s client not great
    * Great HTTP libraries
  * Rust? 
    * 2nd place in terms of measured efficiency, could probably get very close to chromedp
    * Library a little abandoned/forked
    * Good k8s client
    * Rust would be a completely new platform for our team
    * Good HTTP libraries (though not stdlib)
    * Native code, fast startup, lightweight
  * Playwright .NET or JS?
    * ?
  * go-rod
  * Custom implementation relying only on CDP
    * Less dependencies
    * Performance gains to be had
      * Batching CDP commands (very specifically to our usecase)

## Next steps

* Investigate custom implementation (timebox)
  * 01.09.2025 - seems like this doesn't yield the wanted benefits (batching and efficiency). The client has to wait for responses as there is no way to tell chrome to do certain operations serially
* Investigate go-rod (timebox)
  * 02.09.2025 - go-rod seems to be atleast a little buggy, was unable to configure `GenerateTaggedPDF` to false since it deems false as empty during `omitempty` JSON serialization config (browser defaults to true apparantly). I therefore set it to true for both generators during loadtests and it seems like chromedp came out on top, unclear why, would have to understand a lot more about internals. So while chromedp has a lot more complicated interface, it is more efficient (both measured in latency and the initial memory usage it seems)
* Design v3 architecture (what happens in k8s)
* Securiy clarifications
* Build
