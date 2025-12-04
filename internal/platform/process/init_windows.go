package process

import (
	"golang.org/x/sys/windows"
	"log"
	"unsafe"
)

func KillProcessTreeOnClose() {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)

	windows.AssignProcessToJobObject(job, windows.CurrentProcess())
}
