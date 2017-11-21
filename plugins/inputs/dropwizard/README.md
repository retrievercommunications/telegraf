# Dropwizard Input Plugin

The dropwizard plugin gathers metrics from a [Dropwizard](http://www.dropwizard.io/) web application via HTTP.

It also supports gathering metrics from a non-Dropwizard web application that uses the [Dropwizard Metrics](http://metrics.dropwizard.io) library and exposes the library's AdminServlet or MetricsServlet endpoint.

The plugin expects that the web application will serve a JSON representation of all registered metrics via HTTP.

This plugin is an alternate to using Dropwizard Metrics Reporter implementations like the ones from [iZettle](https://github.com/iZettle/dropwizard-metrics-influxdb) and [kickstarter](https://github.com/kickstarter/dropwizard-influxdb-reporter). The features in this plugin are inspired by the above mentioned Reporter libraries.
The main differences with this plugin compared to the Reporters are:

- metrics will be pulled from the Dropwizard application rather than it pushing metrics via the Reporters. This reduces the responsibilities of the web application
- you can change configuration or functionality without restarting the application. This gives Operations greater flexibility to manage the infrastructure without affecting the apps.

### Configuration:

This section contains the default TOML to configure the plugin.  You can
generate it using `telegraf --usage dropwizard`.

```toml
# Read Dropwizard-formatted JSON metrics from one or more HTTP endpoints
[[inputs.dropwizard]]
  ## Works with Dropwizard metrics endpoint out of the box

  ## Multiple URLs from which to read Dropwizard-formatted JSON
  ## Default is "http://localhost:8081/metrics".
  urls = [
    "http://localhost:8081/metrics"
  ]

  ## Optional SSL Config
  # ssl_ca = "/etc/telegraf/ca.pem"
  # ssl_cert = "/etc/telegraf/cert.pem"
  # ssl_key = "/etc/telegraf/key.pem"
  ## Use SSL but skip chain & host verification
  # insecure_skip_verify = false

  ## http request & header timeout
  timeout = "10s"

  ## exclude some built-in metrics
  # namedrop = [
  #  "jvm.classloader*",
  #  "jvm.buffers*",
  #  "jvm.gc*",
  #  "jvm.memory.heap*",
  #  "jvm.memory.non-heap*",
  #  "jvm.memory.pools*",
  #  "jvm.threads*",
  #  "jvm.attribute.uptime",
  #  "jvm.filedescriptor",
  #  "io.dropwizard.jetty.MutableServletContextHandler*",
  #  "org.eclipse.jetty.util*"
  # ]

  ## include only the required fields (applies to all metrics types)
  # fieldpass = [
  #  "count",
  #  "max",
  #  "p999",
  #  "m5_Rate",
  #  "value"
  # ]
```

### Metrics:

The [Dropwizard Metrics](http://metrics.dropwizard.io) library supports 5 metric types and each have a fixed number of fields shown in brackets:

- Gauge (value)
- Counter (count)
- Histogram (count, max, mean, min, p50, p75, p95, p98, p99, p999, stddev)
- Meter (count, m15_rate, m1_rate, m5_rate, mean_rate)
- Timer (count, max, mean, min, p50, p75, p95, p98, p99, p999, stddev, m15_rate, m1_rate, m5_rate, mean_rate)

The metrics will include any that you have added in your application plus some built-in ones (e.g. JVM, jetty and logback). 
You can omit some of the built-in ones by using Telegraf's ```namedrop``` configuration that is available on all input plugins. An example is included in the sample config.
Any metrics that are non-numeric will be dropped.

### Sample Queries:

To plot the memory used by the Dropwizard application:
```
SELECT mean("value") AS "mean_value" FROM "telegraf"."autogen"."jvm.memory.total.used" WHERE time > now() - 5m GROUP BY time(15s) FILL(null)
```

### Example Output:

```
./telegraf --input-filter dropwizard --test
2017/11/22 07:06:40 I! Using config file: /Users/telegraf/.telegraf/telegraf.conf
* Plugin: inputs.dropwizard, Collection 1
> jvm.memory.total.used,host=myhost value=93176816i 1511294800000000000
> jvm.memory.total.max,host=myhost value=1908932607i 1511294800000000000
> jvm.memory.total.committed,host=myhost value=177602560i 1511294800000000000
> jvm.memory.total.init,host=myhost value=136773632i 1511294800000000000
> ch.qos.logback.core.Appender.all,host=myhost count=16i 1511294800000000000
> ch.qos.logback.core.Appender.debug,host=myhost count=0i 1511294800000000000
> ch.qos.logback.core.Appender.error,host=myhost count=0i 1511294800000000000
> ch.qos.logback.core.Appender.trace,host=myhost count=0i 1511294800000000000
> ch.qos.logback.core.Appender.info,host=myhost count=16i 1511294800000000000
> ch.qos.logback.core.Appender.warn,host=myhost count=0i 1511294800000000000
> org.eclipse.jetty.server.HttpConnectionFactory.8081.connections,host=myhost max=48.568033221,count=3i,p999=5.019724599 1511294800000000000
> org.eclipse.jetty.server.HttpConnectionFactory.8080.connections,host=myhost max=0,p999=0,count=0i 1511294800000000000
```

### TODO:

Currently this plugin does the basics of pulling the metrics from Dropwizard JSON/HTTP endpoint and you can use Telegraf's built-in features to determine which metrics and fields get sent to your output. 
It would be nice to have some additional features like the following:

- Group single field metrics (i.e. gauges and counters) with a common prefix into 1 measurement with multiple fields. For example, they are 4 jvm gauge metrics with the common prefix "jvm.memory.total". Instead of the 4 single field measurements, it would create 1 measurement with 4 fields. This would help improve the resulting influxdb schema.

- Pull other information from Dropwizard's AdminServlet like Healthchecks

- Per-metric tags, these could be derived using a naming convention like "measurement.name,tag1=value1,tag2=value2"

- Metric name to Measurement name mapping (i.e. renaming). For example, could support mapping "jvm.memory.total" metrics to "jvm_memory" through configuration
