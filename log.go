package main

func (c *checker) logf(format string, v ...interface{}) {
	if c.log != nil {
		c.log.Printf(format, v...)
	}
}

func (c *checker) logln(v ...interface{}) {
	if c.log != nil {
		c.log.Println(v...)
	}
}
