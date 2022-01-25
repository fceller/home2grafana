/*
 * MIT License
 *
 * Copyright (c) 2022 Frank Celler
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 */

package devices

import (
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
)

type Context struct {
	Root      string
	NetClient *http.Client
	Clog      *logrus.Entry
	loggers   []*logrus.Entry
}

func (c *Context) PushFields(fields logrus.Fields) {
	c.loggers = append(c.loggers, c.Clog)
	c.Clog = c.Clog.WithFields(fields)
}

func (c *Context) PushField(key string, value interface{}) {
	c.loggers = append(c.loggers, c.Clog)
	c.Clog = c.Clog.WithField(key, value)
}

func (c *Context) Pop() {
	n := len(c.loggers)

	if 0 < n {
		c.Clog = c.loggers[n-1]
		c.loggers[n-1] = nil
		c.loggers = c.loggers[:(n - 1)]
	}
}

func (c *Context) Info(args ...interface{}) {
	c.Clog.Info(args...)
}

func (c *Context) Warn(err error, args ...interface{}) {
	c.Clog.WithError(err).Warn(args...)
}

func (c *Context) Fatal(err error, args ...interface{}) {
	c.Clog.WithError(err).Fatal(args...)
}

func (c *Context) Post(url string, contentType string, body io.Reader) (resp *http.Response, err error) {
	return c.NetClient.Post(url, contentType, body)
}
