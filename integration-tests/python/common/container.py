from abc import ABC, abstractmethod


class Container(ABC):
    @abstractmethod
    def id(self):
        pass

    @abstractmethod
    def logs(self) -> str:
        pass

    @abstractmethod
    def stop(self):
        pass

    @abstractmethod
    def kill(self):
        pass

    @abstractmethod
    def remove(self):
        pass

    @abstractmethod
    def is_running(self) -> bool:
        pass

    @abstractmethod
    def exec(self, command, *args, **kwargs) -> (int, str):
        pass

    @abstractmethod
    def ip(self):
        pass

    @abstractmethod
    def port(self):
        pass

class DockerContainer(Container):
    def __init__(self, handle):
        self.handle = handle

    def id(self):
        return self.handle.id[:12]

    def logs(self) -> str:
        return self.handle.logs().decode()

    def stop(self):
        return self.handle.stop()

    def kill(self):
        return self.handle.kill()

    def remove(self):
        return self.handle.remove()

    def is_running(self) -> bool:
        return self.handle.status == 'running'

    def exec(self, command, *args, **kwargs) -> (int, str):
        return self.handle.exec_run(command, *args, **kwargs)

    def ip(self):
        networks = self.handle.attrs['NetworkSettings']['Networks']
        print(f"Networks: {networks}")
        return networks[0]['IPAddress'].replace("'")

    def port(self):
        ports = self.handle.attrs['NetworkSettings']['Ports']
        print(f"Ports: {ports}")
        for k in ports:
            return k.split('/')[0]
